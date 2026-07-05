package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/acl"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
)

var _ resource.Resource = &networkACLRuleResource{}
var _ resource.ResourceWithConfigure = &networkACLRuleResource{}
var _ resource.ResourceWithImportState = &networkACLRuleResource{}

type networkACLRuleResource struct {
	svc aclServiceIface
}

type networkACLRuleResourceModel struct {
	ID          types.String `tfsdk:"id"`
	VPC         types.String `tfsdk:"vpc"`
	ACL         types.String `tfsdk:"acl"`
	Number      types.Int64  `tfsdk:"number"`
	Action      types.String `tfsdk:"action"`
	TrafficType types.String `tfsdk:"traffic_type"`
	Protocol    types.String `tfsdk:"protocol"`
	CIDRList    types.String `tfsdk:"cidr_list"`
	StartPort   types.Int64  `tfsdk:"start_port"`
	EndPort     types.Int64  `tfsdk:"end_port"`
	ICMPType    types.Int64  `tfsdk:"icmp_type"`
	ICMPCode    types.Int64  `tfsdk:"icmp_code"`
	Description types.String `tfsdk:"description"`
}

func NewNetworkACLRuleResource() resource.Resource {
	return &networkACLRuleResource{}
}

func (r *networkACLRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_acl_rule"
}

func (r *networkACLRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a single rule in a ZCP Network ACL. Rules are independent resources (like `aws_network_acl_rule`); `number`, `action`, `traffic_type`, `protocol`, `cidr_list`, ports, and ICMP fields are updated in place. Moving a rule to a different ACL or VPC forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Rule ID (UUID).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"vpc": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Slug of the `zcp_vpc` the ACL belongs to. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"acl": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the `zcp_network_acl` this rule belongs to. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"number": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Rule number (order/priority). Must be unique within the ACL.",
				Validators:          []validator.Int64{int64AtLeastValidator{min: 1}},
			},
			"action": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Rule action: `allow` or `deny`.",
				Validators:          []validator.String{stringOneOfValidator{allowed: []string{"allow", "deny"}}},
			},
			"traffic_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Traffic direction: `ingress` or `egress`.",
				Validators:          []validator.String{stringOneOfValidator{allowed: []string{"ingress", "egress"}}},
			},
			"protocol": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Protocol: `tcp`, `udp`, `icmp`, or `all`.",
				Validators:          []validator.String{stringOneOfValidator{allowed: []string{"tcp", "udp", "icmp", "all"}}},
			},
			"cidr_list": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "CIDR the rule applies to (e.g. `0.0.0.0/0`).",
			},
			"start_port": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Start of the port range (required for `tcp`/`udp`).",
			},
			"end_port": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "End of the port range (required for `tcp`/`udp`).",
			},
			"icmp_type": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "ICMP type (required for `icmp`).",
			},
			"icmp_code": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "ICMP code (required for `icmp`).",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Human-readable description of the rule.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *networkACLRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = acl.NewService(pd.Client)
}

// toRuleRequest builds the API request from the model.
func (r *networkACLRuleResource) toRuleRequest(model networkACLRuleResourceModel) acl.RuleCreateRequest {
	req := acl.RuleCreateRequest{
		Number:      int(model.Number.ValueInt64()),
		Protocol:    model.Protocol.ValueString(),
		CIDRList:    model.CIDRList.ValueString(),
		Action:      model.Action.ValueString(),
		TrafficType: model.TrafficType.ValueString(),
		Description: model.Description.ValueString(),
	}
	if !model.StartPort.IsNull() && !model.StartPort.IsUnknown() {
		v := int(model.StartPort.ValueInt64())
		req.StartPort = &v
	}
	if !model.EndPort.IsNull() && !model.EndPort.IsUnknown() {
		v := int(model.EndPort.ValueInt64())
		req.EndPort = &v
	}
	if !model.ICMPType.IsNull() && !model.ICMPType.IsUnknown() {
		v := int(model.ICMPType.ValueInt64())
		req.ICMPType = &v
	}
	if !model.ICMPCode.IsNull() && !model.ICMPCode.IsUnknown() {
		v := int(model.ICMPCode.ValueInt64())
		req.ICMPCode = &v
	}
	return req
}

func (r *networkACLRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model networkACLRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_network_acl_rule cannot be created: bearer_token is missing.")
		return
	}

	vpcSlug := model.VPC.ValueString()
	aclID := model.ACL.ValueString()
	number := int(model.Number.ValueInt64())

	// The API requires a non-empty description; default it.
	if model.Description.IsNull() || model.Description.IsUnknown() || model.Description.ValueString() == "" {
		model.Description = types.StringValue(fmt.Sprintf("rule %d", number))
	}

	if err := r.svc.CreateRule(ctx, vpcSlug, aclID, r.toRuleRequest(model)); err != nil {
		resp.Diagnostics.AddError("Failed to create ACL rule", err.Error())
		return
	}

	// CreateRule does not return the new rule, so resolve its ID by rule number
	// (unique within the ACL).
	rules, err := r.svc.ListRules(ctx, vpcSlug, aclID)
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve created ACL rule", err.Error())
		return
	}
	found := false
	for _, rule := range rules {
		if rule.Number == number {
			model.ID = types.StringValue(rule.ID)
			found = true
			break
		}
	}
	if !found {
		resp.Diagnostics.AddError("Failed to resolve created ACL rule", fmt.Sprintf("rule number %d not found in ACL %s after creation", number, aclID))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *networkACLRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model networkACLRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_network_acl_rule cannot be read: bearer_token is missing.")
		return
	}

	rules, err := r.svc.ListRules(ctx, model.VPC.ValueString(), model.ACL.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read ACL rule", err.Error())
		return
	}
	id := model.ID.ValueString()
	for _, rule := range rules {
		if rule.ID == id {
			model.Number = types.Int64Value(int64(rule.Number))
			// The API accepts lowercase action/traffic_type but echoes them
			// capitalized ("Allow"/"Ingress"); normalize to the canonical lowercase
			// form so a re-plan is zero-diff.
			model.Action = types.StringValue(strings.ToLower(rule.Action))
			model.TrafficType = types.StringValue(strings.ToLower(rule.TrafficType))
			model.Protocol = types.StringValue(strings.ToLower(rule.Protocol))
			model.CIDRList = types.StringValue(rule.CIDRList)
			if rule.Description != "" {
				model.Description = types.StringValue(rule.Description)
			}
			model.StartPort = parsePort(rule.StartPort, model.StartPort)
			model.EndPort = parsePort(rule.EndPort, model.EndPort)
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

// parsePort converts the API's string port into an Int64, preserving the current
// value when the API returns an empty/non-numeric string (e.g. for icmp/all).
func parsePort(api string, current types.Int64) types.Int64 {
	if api == "" {
		return current
	}
	n, err := strconv.ParseInt(api, 10, 64)
	if err != nil {
		return current
	}
	return types.Int64Value(n)
}

func (r *networkACLRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model networkACLRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state networkACLRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_network_acl_rule cannot be updated: bearer_token is missing.")
		return
	}

	if err := r.svc.UpdateRule(ctx, state.VPC.ValueString(), state.ACL.ValueString(), state.ID.ValueString(), r.toRuleRequest(model)); err != nil {
		resp.Diagnostics.AddError("Failed to update ACL rule", err.Error())
		return
	}
	model.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *networkACLRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model networkACLRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_network_acl_rule cannot be deleted: bearer_token is missing.")
		return
	}
	if err := r.svc.DeleteRule(ctx, model.VPC.ValueString(), model.ACL.ValueString(), model.ID.ValueString()); err != nil &&
		!apierrors.IsNotFound(err) && !apierrors.IsResourceNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete ACL rule", err.Error())
	}
}

// ImportState accepts "<vpc-slug>/<acl-id>/<rule-id>".
func (r *networkACLRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"vpc", "acl", "id"}
	importPositional(ctx, req, resp, fields, 3, "<vpc-slug>/<acl-id>/<rule-id>")
}
