// Package provider implements the ZCP Terraform provider.
package provider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/zsoftly/zcp-cli/pkg/httpclient"
)

const defaultAPIURL = "https://api.zcp.zsoftly.ca/api"

var _ provider.Provider = &ZCPProvider{}

type ZCPProvider struct {
	version string
}

type ZCPProviderModel struct {
	BearerToken    types.String `tfsdk:"bearer_token"`
	APIURL         types.String `tfsdk:"api_url"`
	DefaultProject types.String `tfsdk:"default_project"`
	Profile        types.String `tfsdk:"profile"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &ZCPProvider{version: version}
	}
}

func (p *ZCPProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "zcp"
	resp.Version = p.version
}

func (p *ZCPProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The ZCP provider manages ZSoftly Cloud Platform resources.",
		Attributes: map[string]schema.Attribute{
			"bearer_token": schema.StringAttribute{
				MarkdownDescription: "ZCP API bearer token. May also be set via `ZCP_BEARER_TOKEN`.",
				Optional:            true,
				Sensitive:           true,
			},
			"api_url": schema.StringAttribute{
				MarkdownDescription: "ZCP API base URL. Defaults to `" + defaultAPIURL + "`. May also be set via `ZCP_API_URL`.",
				Optional:            true,
			},
			"default_project": schema.StringAttribute{
				MarkdownDescription: "Default project slug applied to resources that do not specify a project. May also be set via `ZCP_PROJECT`.",
				Optional:            true,
			},
			"profile": schema.StringAttribute{
				MarkdownDescription: "ZCP CLI profile whose API URL and default project apply. The profile's bearer token is used after bearer_token and ZCP_BEARER_TOKEN. May also be set via ZCP_PROFILE.",
				Optional:            true,
			},
		},
	}
}

func (p *ZCPProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config ZCPProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.BearerToken.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("bearer_token"),
			"Unknown ZCP bearer token",
			"The bearer_token value must be known during provider configuration.",
		)
		return
	}
	if config.APIURL.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_url"),
			"Unknown ZCP API URL",
			"The api_url value must be known during provider configuration.",
		)
		return
	}
	if config.DefaultProject.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("default_project"),
			"Unknown ZCP default project",
			"The default_project value must be known during provider configuration.",
		)
		return
	}

	var selectedProfile cliProfile
	profileName := ""
	profileExplicit := false
	if config.Profile.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("profile"),
			"Unknown ZCP CLI profile",
			"The profile value must be known during provider configuration.",
		)
		return
	}
	if !config.Profile.IsNull() && config.Profile.ValueString() != "" {
		profileName = config.Profile.ValueString()
		profileExplicit = true
	} else if envProfile := os.Getenv("ZCP_PROFILE"); envProfile != "" {
		profileName = envProfile
		profileExplicit = true
	}
	if profileExplicit {
		var err error
		selectedProfile, err = loadCLIProfile(profileName)
		if err != nil {
			resp.Diagnostics.AddError("Invalid ZCP CLI profile", err.Error())
			return
		}
	} else {
		activeName, activeProfile, err := activeCLIProfile()
		if err != nil {
			// A configured token remains usable when an old CLI configuration
			// refers to a profile that has since been removed.
			hasConfiguredToken := (!config.BearerToken.IsNull() && !config.BearerToken.IsUnknown() && config.BearerToken.ValueString() != "") || os.Getenv("ZCP_BEARER_TOKEN") != ""
			if !hasConfiguredToken || !errors.Is(err, errActiveCLIProfileNotFound) {
				resp.Diagnostics.AddError("Invalid active ZCP CLI profile", err.Error())
				return
			}
		} else if activeName != "" {
			profileName = activeName
			selectedProfile = activeProfile
		}
	}

	// bearer_token: HCL attribute > environment variable > selected or active CLI profile.
	bearerToken := ""
	credentialSource := ""
	if !config.BearerToken.IsNull() && !config.BearerToken.IsUnknown() {
		bearerToken = config.BearerToken.ValueString()
		credentialSource = "provider configuration"
	} else if envToken := os.Getenv("ZCP_BEARER_TOKEN"); envToken != "" {
		bearerToken = envToken
		credentialSource = "ZCP_BEARER_TOKEN"
	} else if profileExplicit {
		bearerToken = selectedProfile.BearerToken
		credentialSource = fmt.Sprintf("ZCP CLI profile %q", profileName)
	} else if profileName != "" {
		bearerToken = selectedProfile.BearerToken
		credentialSource = fmt.Sprintf("active ZCP CLI profile %q", profileName)
	}
	if bearerToken == "" {
		resp.Diagnostics.AddError(
			"Missing ZCP bearer token",
			"Set bearer_token in the provider block, export ZCP_BEARER_TOKEN, or configure a ZCP CLI profile.",
		)
		return
	}
	tflog.Debug(ctx, "Configured ZCP authentication", map[string]any{"credential_source": credentialSource})

	// api_url: HCL attribute > env var > CLI profile > built-in default.
	apiURL := defaultAPIURL
	if v := os.Getenv("ZCP_API_URL"); v != "" {
		apiURL = v
	}
	if selectedProfile.APIURL != "" && os.Getenv("ZCP_API_URL") == "" {
		apiURL = selectedProfile.APIURL
	}
	if !config.APIURL.IsNull() && !config.APIURL.IsUnknown() {
		apiURL = config.APIURL.ValueString()
	}

	// Validate api_url is a parseable absolute URL.
	if u, err := url.ParseRequestURI(apiURL); err != nil || u.Host == "" {
		resp.Diagnostics.AddError(
			"Invalid api_url",
			fmt.Sprintf("%q is not a valid URL: must be an absolute HTTP/HTTPS URL.", apiURL),
		)
		return
	}

	// default_project: HCL attribute > env var > CLI profile.
	defaultProject := os.Getenv("ZCP_PROJECT")
	if defaultProject == "" {
		defaultProject = selectedProfile.Project
	}
	if !config.DefaultProject.IsNull() && !config.DefaultProject.IsUnknown() {
		defaultProject = config.DefaultProject.ValueString()
	}

	pd := &ProviderData{
		Client: httpclient.New(httpclient.Options{
			BaseURL:     apiURL,
			BearerToken: bearerToken,
			Timeout:     5 * time.Minute,
		}),
		DefaultProject: defaultProject,
	}
	resp.ResourceData = pd
	resp.DataSourceData = pd
}

func (p *ZCPProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewSSHKeyResource,
		NewNetworkResource,
		NewVPCResource,
		NewFirewallRuleResource,
		NewIPAddressResource,
		NewVPNUserResource,
		NewVPNCustomerGatewayResource,
		NewPortForwardResource,
		NewVPCVPNGatewayResource,
		NewInstanceResource,
		NewVolumeResource,
		NewKubernetesClusterResource,
		NewNetworkACLResource,
		NewNetworkACLRuleResource,
		NewIPAssociationResource,
		NewEgressRuleResource,
		NewAffinityGroupResource,
		NewDNSDomainResource,
		NewDNSRecordResource,
		NewLoadBalancerResource,
		NewLoadBalancerRuleResource,
		NewLoadBalancerAttachmentResource,
		NewObjectStorageResource,
		NewObjectStorageKeyResource,
		NewObjectStorageBucketResource,
		NewObjectStorageBucketEncryptionResource,
		NewObjectStorageBucketVersioningResource,
		NewObjectStorageBucketPolicyResource,
		NewObjectStorageBucketTaggingResource,
		NewObjectStorageBucketLifecycleResource,
		NewObjectStorageBucketCORSResource,
		NewVMSnapshotResource,
		NewVMBackupResource,
		NewVMBackupScheduleResource,
		NewVolumeSnapshotScheduleResource,
		NewVolumeSnapshotResource,
		NewVolumeBackupResource,
		NewAutoscaleGroupResource,
		NewAutoscalePolicyResource,
		NewAutoscaleConditionResource,
		NewISOResource,
		NewAccountTemplateResource,
		NewProjectResource,
		NewSubUserResource,
		NewRoleResource,
		NewBudgetAlertResource,
		NewRemoteAccessVPNResource,
	}
}

func (p *ZCPProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewProjectDataSource,
		NewRegionDataSource,
		NewTemplateDataSource,
		NewPlanDataSource,
		NewStorageCategoryDataSource,
		NewBillingCycleDataSource,
		NewNetworkDataSource,
		NewVPCDataSource,
		NewSSHKeyDataSource,
		NewKubernetesVersionDataSource,
		NewInstanceDataSource,
		NewVolumeDataSource,
		NewLoadBalancerDataSource,
		NewPermissionsDataSource,
	}
}
