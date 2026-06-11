package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/vpc"
)

var _ resource.Resource = &vpcVPNGatewayResource{}
var _ resource.ResourceWithConfigure = &vpcVPNGatewayResource{}
var _ resource.ResourceWithImportState = &vpcVPNGatewayResource{}

type vpcVPNGatewayServiceIface interface {
	ListVPNGateways(ctx context.Context, vpcSlug string) ([]vpc.VPNGateway, error)
	CreateVPNGateway(ctx context.Context, vpcSlug string) (*vpc.VPNGateway, error)
	DeleteVPNGateway(ctx context.Context, vpcSlug, gatewayID string) error
}

type vpcVPNGatewayResource struct {
	svc vpcVPNGatewayServiceIface
}

type vpcVPNGatewayResourceModel struct {
	ID       types.String   `tfsdk:"id"`
	VPC      types.String   `tfsdk:"vpc"`
	PublicIP types.String   `tfsdk:"public_ip"`
	Status   types.String   `tfsdk:"status"`
	ZoneName types.String   `tfsdk:"zone_name"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func NewVPCVPNGatewayResource() resource.Resource {
	return &vpcVPNGatewayResource{}
}

func (r *vpcVPNGatewayResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vpc_vpn_gateway"
}

func (r *vpcVPNGatewayResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP VPN gateway attached to a VPC.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "VPN gateway slug (unique identifier).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"vpc": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Parent VPC slug. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"public_ip": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Public IP address assigned to the VPN gateway.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current status of the VPN gateway.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"zone_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Zone in which the VPN gateway is deployed.",
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

func (r *vpcVPNGatewayResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = vpc.NewService(pd.Client)
}

func (r *vpcVPNGatewayResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model vpcVPNGatewayResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vpc_vpn_gateway cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	gw, err := r.svc.CreateVPNGateway(ctx, model.VPC.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create VPN gateway", err.Error())
		return
	}

	model.ID = types.StringValue(gw.ID)
	// Always set concrete values; these may be empty initially and populated after
	// the gateway finishes provisioning (visible on subsequent reads).
	model.PublicIP = types.StringValue(gw.PublicIP)
	model.Status = types.StringValue("")
	model.ZoneName = types.StringValue("")
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *vpcVPNGatewayResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model vpcVPNGatewayResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vpc_vpn_gateway cannot be read: bearer_token is missing.")
		return
	}

	gateways, err := r.svc.ListVPNGateways(ctx, model.VPC.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read VPN gateway", err.Error())
		return
	}

	id := model.ID.ValueString()
	for _, gw := range gateways {
		if gw.ID == id {
			if gw.PublicIP != "" {
				model.PublicIP = types.StringValue(gw.PublicIP)
			}
			// status and zone_name are not returned by the API; preserved from state.
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *vpcVPNGatewayResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are ForceNew; Terraform never invokes this method.
}

func (r *vpcVPNGatewayResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model vpcVPNGatewayResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vpc_vpn_gateway cannot be deleted: bearer_token is missing.")
		return
	}
	deleteTimeout, diags := model.Timeouts.Delete(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	vpcSlug := model.VPC.ValueString()
	gwID := model.ID.ValueString()
	err := r.svc.DeleteVPNGateway(ctx, vpcSlug, gwID)
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete VPN gateway", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		gateways, err := r.svc.ListVPNGateways(ctx, vpcSlug)
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for _, gw := range gateways {
			if gw.ID == gwID {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError("VPN gateway deletion did not complete", err.Error())
	}
}

func (r *vpcVPNGatewayResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", `Expected format: <vpc_slug>/<gateway_slug>`)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vpc"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
