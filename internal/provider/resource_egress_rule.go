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
	"github.com/zsoftly/zcp-cli/pkg/api/egress"
)

var _ resource.Resource = &egressRuleResource{}
var _ resource.ResourceWithConfigure = &egressRuleResource{}
var _ resource.ResourceWithImportState = &egressRuleResource{}

type egressServiceIface interface {
	List(ctx context.Context, networkSlug string) ([]egress.EgressRule, error)
	Create(ctx context.Context, req egress.CreateRequest) (*egress.EgressRule, error)
	Delete(ctx context.Context, networkSlug string, ruleID string) error
}

type egressRuleResource struct {
	svc egressServiceIface
}

type egressRuleResourceModel struct {
	ID        types.String   `tfsdk:"id"`
	Network   types.String   `tfsdk:"network"`
	Protocol  types.String   `tfsdk:"protocol"`
	CIDR      types.String   `tfsdk:"cidr"`
	StartPort types.String   `tfsdk:"start_port"`
	EndPort   types.String   `tfsdk:"end_port"`
	ICMPType  types.String   `tfsdk:"icmp_type"`
	ICMPCode  types.String   `tfsdk:"icmp_code"`
	State     types.String   `tfsdk:"state"`
	Timeouts  timeouts.Value `tfsdk:"timeouts"`
}

func NewEgressRuleResource() resource.Resource {
	return &egressRuleResource{}
}

func (r *egressRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_egress_rule"
}

func (r *egressRuleResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP egress firewall rule on a network.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Egress rule unique identifier.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"network": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Parent network slug the rule applies to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"protocol": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Protocol for the rule (e.g. `tcp`, `udp`, `icmp`, `all`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"cidr": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Destination CIDR the rule allows traffic to (e.g. `0.0.0.0/0`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"start_port": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Start of the port range (e.g. `443`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"end_port": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "End of the port range (e.g. `443`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"icmp_type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "ICMP type (required for `icmp`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"icmp_code": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "ICMP code (required for `icmp`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current state of the egress rule.",
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

func (r *egressRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = egress.NewService(pd.Client)
}

func (r *egressRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model egressRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_egress_rule cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	createReq := egress.CreateRequest{
		NetworkSlug: model.Network.ValueString(),
		Protocol:    model.Protocol.ValueString(),
	}
	if !model.CIDR.IsNull() && !model.CIDR.IsUnknown() {
		createReq.CIDR = model.CIDR.ValueString()
	}
	if !model.StartPort.IsNull() && !model.StartPort.IsUnknown() {
		createReq.StartPort = model.StartPort.ValueString()
	}
	if !model.EndPort.IsNull() && !model.EndPort.IsUnknown() {
		createReq.EndPort = model.EndPort.ValueString()
	}
	if !model.ICMPType.IsNull() && !model.ICMPType.IsUnknown() {
		createReq.ICMPType = model.ICMPType.ValueString()
	}
	if !model.ICMPCode.IsNull() && !model.ICMPCode.IsUnknown() {
		createReq.ICMPCode = model.ICMPCode.ValueString()
	}

	rule, err := r.svc.Create(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create egress rule", err.Error())
		return
	}

	model.ID = types.StringValue(rule.ID)
	if rule.Status != "" {
		model.State = types.StringValue(rule.Status)
	} else {
		// state is Computed; an unknown value after Create fails the apply.
		model.State = types.StringNull()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *egressRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model egressRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_egress_rule cannot be read: bearer_token is missing.")
		return
	}

	rules, err := r.svc.List(ctx, model.Network.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read egress rule", err.Error())
		return
	}

	id := model.ID.ValueString()
	for _, rule := range rules {
		if rule.ID == id {
			model.Protocol = types.StringValue(rule.Protocol)
			// The API echoes the requested destination CIDR in destcidr_list;
			// the top-level cidr field is the network's own CIDR.
			if rule.DestCIDR != "" {
				model.CIDR = types.StringValue(rule.DestCIDR)
			}
			if rule.StartPort != "" {
				model.StartPort = types.StringValue(rule.StartPort)
			}
			if rule.EndPort != "" {
				model.EndPort = types.StringValue(rule.EndPort)
			}
			if rule.ICMPType != "" {
				model.ICMPType = types.StringValue(rule.ICMPType)
			}
			if rule.ICMPCode != "" {
				model.ICMPCode = types.StringValue(rule.ICMPCode)
			}
			if rule.Status != "" {
				model.State = types.StringValue(rule.Status)
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *egressRuleResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are ForceNew; Terraform never invokes this method.
}

func (r *egressRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model egressRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_egress_rule cannot be deleted: bearer_token is missing.")
		return
	}

	deleteTimeout, diags := model.Timeouts.Delete(ctx, 2*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	networkSlug := model.Network.ValueString()
	ruleID := model.ID.ValueString()
	err := r.svc.Delete(deleteCtx, networkSlug, ruleID)
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete egress rule", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		rules, err := r.svc.List(ctx, networkSlug)
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
		resp.Diagnostics.AddError("Egress rule deletion did not complete", err.Error())
	}
}

func (r *egressRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Expected format: "network-slug/rule-id"
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected format \"<network>/<rule_id>\", got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("network"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
