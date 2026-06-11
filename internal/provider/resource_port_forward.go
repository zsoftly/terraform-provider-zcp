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
	"github.com/zsoftly/zcp-cli/pkg/api/portforward"
)

var _ resource.Resource = &portForwardResource{}
var _ resource.ResourceWithConfigure = &portForwardResource{}
var _ resource.ResourceWithImportState = &portForwardResource{}

type portForwardServiceIface interface {
	List(ctx context.Context, ipSlug string) ([]portforward.PortForwardRule, error)
	Create(ctx context.Context, ipSlug string, req portforward.CreateRequest) (*portforward.PortForwardRule, error)
	Delete(ctx context.Context, ipSlug string, ruleID string) error
}

type portForwardResource struct {
	svc portForwardServiceIface
}

type portForwardResourceModel struct {
	ID               types.String   `tfsdk:"id"`
	IPAddress        types.String   `tfsdk:"ip_address"`
	Protocol         types.String   `tfsdk:"protocol"`
	PublicStartPort  types.String   `tfsdk:"public_start_port"`
	PublicEndPort    types.String   `tfsdk:"public_end_port"`
	PrivateStartPort types.String   `tfsdk:"private_start_port"`
	PrivateEndPort   types.String   `tfsdk:"private_end_port"`
	VirtualMachine   types.String   `tfsdk:"virtual_machine"`
	State            types.String   `tfsdk:"state"`
	Timeouts         timeouts.Value `tfsdk:"timeouts"`
}

func NewPortForwardResource() resource.Resource {
	return &portForwardResource{}
}

func (r *portForwardResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_port_forward"
}

func (r *portForwardResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP port forwarding rule on a public IP address.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Port forwarding rule UUID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"ip_address": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Parent IP address slug (e.g. \"1036521143\"). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"protocol": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "IP protocol (e.g. \"tcp\", \"udp\"). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"public_start_port": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "First public port in the range. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"public_end_port": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Last public port in the range. Omit for a single-port rule. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"private_start_port": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "First private (VM-side) port in the range. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"private_end_port": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Last private (VM-side) port in the range. Omit for a single-port rule. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"virtual_machine": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "VM slug to forward traffic to. Write-only — the API returns a VM object, not the slug. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current state of the port forwarding rule.",
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

func (r *portForwardResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = portforward.NewService(pd.Client)
}

func (r *portForwardResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model portForwardResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_port_forward cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	createReq := portforward.CreateRequest{
		Protocol:         model.Protocol.ValueString(),
		PublicStartPort:  model.PublicStartPort.ValueString(),
		PrivateStartPort: model.PrivateStartPort.ValueString(),
		VirtualMachine:   model.VirtualMachine.ValueString(),
	}
	if !model.PublicEndPort.IsNull() && !model.PublicEndPort.IsUnknown() {
		createReq.PublicEndPort = model.PublicEndPort.ValueString()
	}
	if !model.PrivateEndPort.IsNull() && !model.PrivateEndPort.IsUnknown() {
		createReq.PrivateEndPort = model.PrivateEndPort.ValueString()
	}

	rule, err := r.svc.Create(ctx, model.IPAddress.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create port forwarding rule", err.Error())
		return
	}

	model.ID = types.StringValue(rule.ID)
	if rule.State != "" {
		model.State = types.StringValue(rule.State)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *portForwardResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model portForwardResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_port_forward cannot be read: bearer_token is missing.")
		return
	}

	rules, err := r.svc.List(ctx, model.IPAddress.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read port forwarding rules", err.Error())
		return
	}

	id := model.ID.ValueString()
	for _, rule := range rules {
		if rule.ID == id {
			if rule.State != "" {
				model.State = types.StringValue(rule.State)
			}
			// virtual_machine is write-only; rule.VirtualMachine is a VMRef struct,
			// not a plain string — preserve the slug from state.
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *portForwardResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are RequiresReplace; Terraform never invokes this method.
}

func (r *portForwardResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model portForwardResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_port_forward cannot be deleted: bearer_token is missing.")
		return
	}

	deleteTimeout, diags := model.Timeouts.Delete(ctx, 2*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	ipSlug := model.IPAddress.ValueString()
	ruleID := model.ID.ValueString()
	err := r.svc.Delete(ctx, ipSlug, ruleID)
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete port forwarding rule", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		rules, err := r.svc.List(ctx, ipSlug)
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for _, r := range rules {
			if r.ID == ruleID {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError("Port forwarding rule deletion did not complete", err.Error())
	}
}

func (r *portForwardResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", `Expected format: <ip_address_slug>/<rule_id>`)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip_address"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
