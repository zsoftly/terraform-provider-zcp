package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/ipaddress"
)

var _ resource.Resource = &ipAddressResource{}
var _ resource.ResourceWithConfigure = &ipAddressResource{}
var _ resource.ResourceWithImportState = &ipAddressResource{}

type ipAddressServiceIface interface {
	Allocate(ctx context.Context, req ipaddress.CreateRequest) (*ipaddress.IPAddress, error)
	List(ctx context.Context, vpcSlug string) ([]ipaddress.IPAddress, error)
	Release(ctx context.Context, slug string) error
}

type ipAddressResource struct {
	svc            ipAddressServiceIface
	defaultProject string
}

type ipAddressResourceModel struct {
	ID           types.String   `tfsdk:"id"`
	Plan         types.String   `tfsdk:"plan"`
	BillingCycle types.String   `tfsdk:"billing_cycle"`
	VPC          types.String   `tfsdk:"vpc"`
	Network      types.String   `tfsdk:"network"`
	Project      types.String   `tfsdk:"project"`
	IPAddress    types.String   `tfsdk:"ip_address"`
	Type         types.String   `tfsdk:"type"`
	Timeouts     timeouts.Value `tfsdk:"timeouts"`
}

func NewIPAddressResource() resource.Resource {
	return &ipAddressResource{}
}

func (r *ipAddressResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ip_address"
}

func (r *ipAddressResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP public IP address.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "IP address slug (unique identifier).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"plan": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Plan slug for the IP address (e.g. `public-ip-1`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"billing_cycle": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Billing cycle (e.g. `hourly`, `monthly`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"vpc": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "VPC slug to associate with the IP address.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"network": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Network slug to associate with the IP address.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project slug. Inherits from the provider `default_project` if omitted.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"ip_address": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The allocated public IP address.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "IP address type (e.g. `Public`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
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

func (r *ipAddressResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = ipaddress.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

func (r *ipAddressResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model ipAddressResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_ip_address cannot be created: bearer_token is missing.")
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

	ip, err := r.svc.Allocate(ctx, ipaddress.CreateRequest{
		VPC:          model.VPC.ValueString(),
		Network:      model.Network.ValueString(),
		Plan:         model.Plan.ValueString(),
		BillingCycle: model.BillingCycle.ValueString(),
		Project:      project,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to allocate IP address", err.Error())
		return
	}

	model.ID = types.StringValue(ip.Slug)
	if ip.IPAddress != "" {
		model.IPAddress = types.StringValue(ip.IPAddress)
	}
	if ip.Type != "" {
		model.Type = types.StringValue(ip.Type)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *ipAddressResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model ipAddressResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_ip_address cannot be read: bearer_token is missing.")
		return
	}

	ips, err := r.svc.List(ctx, "")
	if err != nil {
		resp.Diagnostics.AddError("Failed to read IP address", err.Error())
		return
	}

	slug := model.ID.ValueString()
	for _, ip := range ips {
		if ip.Slug == slug {
			if ip.IPAddress != "" {
				model.IPAddress = types.StringValue(ip.IPAddress)
			}
			if ip.Type != "" {
				model.Type = types.StringValue(ip.Type)
			}
			// vpc, network, plan, billing_cycle, project are write-only (not in API response); preserved from state.
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *ipAddressResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are ForceNew; Terraform never invokes this method.
}

func (r *ipAddressResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model ipAddressResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_ip_address cannot be deleted: bearer_token is missing.")
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
	err := r.svc.Release(ctx, slug)
	if err != nil && !apierrors.IsNotFound(err) && !apierrors.IsResourceNotFound(err) {
		resp.Diagnostics.AddError("Failed to release IP address", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		ips, err := r.svc.List(ctx, "")
		if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for _, ip := range ips {
			if ip.Slug == slug {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError("IP address release did not complete", err.Error())
	}
}

func (r *ipAddressResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
