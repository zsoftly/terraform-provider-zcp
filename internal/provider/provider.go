// Package provider implements the ZCP Terraform provider.
package provider

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
		},
	}
}

func (p *ZCPProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config ZCPProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// bearer_token: HCL attribute takes precedence over env var.
	bearerToken := os.Getenv("ZCP_BEARER_TOKEN")
	if !config.BearerToken.IsNull() && !config.BearerToken.IsUnknown() {
		bearerToken = config.BearerToken.ValueString()
	}
	if bearerToken == "" {
		resp.Diagnostics.AddError(
			"Missing ZCP bearer token",
			"Set bearer_token in the provider block or export ZCP_BEARER_TOKEN.",
		)
		return
	}

	// api_url: HCL attribute > env var > built-in default.
	apiURL := defaultAPIURL
	if v := os.Getenv("ZCP_API_URL"); v != "" {
		apiURL = v
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

	// default_project: HCL attribute > env var.
	defaultProject := os.Getenv("ZCP_PROJECT")
	if !config.DefaultProject.IsNull() && !config.DefaultProject.IsUnknown() {
		defaultProject = config.DefaultProject.ValueString()
	}

	pd := &ProviderData{
		Client: httpclient.New(httpclient.Options{
			BaseURL:     apiURL,
			BearerToken: bearerToken,
		}),
		DefaultProject: defaultProject,
	}
	resp.ResourceData = pd
	resp.DataSourceData = pd
}

func (p *ZCPProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{}
}

func (p *ZCPProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewProjectDataSource,
		NewRegionDataSource,
		NewTemplateDataSource,
		NewPlanDataSource,
	}
}
