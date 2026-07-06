package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/dns"
)

// defaultDNSProvider is sent when the config omits dns_provider; it matches the
// platform's managed PowerDNS deployment (the only provider ZCP offers today).
const defaultDNSProvider = "PowerDNS"

var _ resource.Resource = &dnsDomainResource{}
var _ resource.ResourceWithConfigure = &dnsDomainResource{}
var _ resource.ResourceWithImportState = &dnsDomainResource{}

// dnsServiceIface is shared by zcp_dns_domain and zcp_dns_record.
type dnsServiceIface interface {
	Show(ctx context.Context, slug string) (*dns.Domain, error)
	Create(ctx context.Context, req dns.CreateDomainRequest) (*dns.Domain, error)
	Delete(ctx context.Context, slug string) error
	CreateRecord(ctx context.Context, domainSlug string, req dns.CreateRecordRequest) (*dns.Domain, error)
	DeleteRecord(ctx context.Context, domainSlug string, recordID int) error
}

type dnsDomainResource struct {
	svc            dnsServiceIface
	defaultProject string
}

type dnsDomainResourceModel struct {
	ID            types.String   `tfsdk:"id"`
	Name          types.String   `tfsdk:"name"`
	DNSProvider   types.String   `tfsdk:"dns_provider"`
	CloudProvider types.String   `tfsdk:"cloud_provider"`
	Region        types.String   `tfsdk:"region"`
	Project       types.String   `tfsdk:"project"`
	Status        types.Bool     `tfsdk:"status"`
	Timeouts      timeouts.Value `tfsdk:"timeouts"`
}

func NewDNSDomainResource() resource.Resource {
	return &dnsDomainResource{}
}

func (r *dnsDomainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_domain"
}

func (r *dnsDomainResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP DNS domain (zone). Add records with `zcp_dns_record`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "DNS domain slug.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Fully qualified domain name (e.g. `example.com`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"dns_provider": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "DNS provider backing the zone. Defaults to `" + defaultDNSProvider + "`. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"cloud_provider": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cloud provider slug (e.g. `zsoftly`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Region slug (e.g. `yow-1`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project slug. Inherits from the provider `default_project` if omitted. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"status": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the domain is active.",
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Delete: true,
			}),
		},
	}
}

func (r *dnsDomainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = dns.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

func (r *dnsDomainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model dnsDomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_dns_domain cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	project := r.defaultProject
	if !model.Project.IsNull() && !model.Project.IsUnknown() {
		project = model.Project.ValueString()
	}
	dnsProvider := defaultDNSProvider
	if !model.DNSProvider.IsNull() && !model.DNSProvider.IsUnknown() {
		dnsProvider = model.DNSProvider.ValueString()
	}

	domain, err := r.svc.Create(ctx, dns.CreateDomainRequest{
		Name:          model.Name.ValueString(),
		Project:       project,
		DNSProvider:   dnsProvider,
		CloudProvider: model.CloudProvider.ValueString(),
		Region:        model.Region.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create DNS domain", err.Error())
		return
	}

	model.ID = types.StringValue(domain.Slug)
	model.Status = types.BoolValue(domain.Status)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *dnsDomainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model dnsDomainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_dns_domain cannot be read: bearer_token is missing.")
		return
	}

	domain, err := r.svc.Show(ctx, model.ID.ValueString())
	if isBackendNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read DNS domain", err.Error())
		return
	}

	model.Name = types.StringValue(domain.Name)
	if domain.DNSProvider != "" {
		model.DNSProvider = types.StringValue(domain.DNSProvider)
	}
	model.Status = types.BoolValue(domain.Status)
	// cloud_provider, region, and project are write-only; preserved from state.
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *dnsDomainResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are ForceNew; Terraform never invokes this method.
}

func (r *dnsDomainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model dnsDomainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_dns_domain cannot be deleted: bearer_token is missing.")
		return
	}

	deleteTimeout, diags := model.Timeouts.Delete(ctx, 2*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	slug := model.ID.ValueString()
	err := r.svc.Delete(deleteCtx, slug)
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete DNS domain", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		_, err := r.svc.Show(ctx, slug)
		if isBackendNotFound(err) {
			return false, nil
		}
		return err == nil, err
	}); err != nil {
		resp.Diagnostics.AddError("DNS domain deletion did not complete", err.Error())
	}
}

// ImportState accepts a composite ID so the write-only region/cloud_provider/project
// are seeded for a zero-diff plan after import. Format:
//
//	<slug>/<region>/<cloud_provider>[/<project>]
//
// name, dns_provider, and status come from the subsequent Read.
func (r *dnsDomainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"id", "region", "cloud_provider", "project"}
	importPositional(ctx, req, resp, fields, 3, "<slug>/<region>/<cloud_provider>[/<project>]")
}
