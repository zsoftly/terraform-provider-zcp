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
	"github.com/zsoftly/zcp-cli/pkg/api/firewall"
)

var _ resource.Resource = &firewallRuleResource{}
var _ resource.ResourceWithConfigure = &firewallRuleResource{}
var _ resource.ResourceWithImportState = &firewallRuleResource{}

type firewallServiceIface interface {
	List(ctx context.Context, ipSlug string) ([]firewall.FirewallRule, error)
	Create(ctx context.Context, ipSlug string, req firewall.CreateRequest) (*firewall.FirewallRule, error)
	Delete(ctx context.Context, ipSlug string, ruleID string) error
}

type firewallRuleResource struct {
	svc firewallServiceIface
}

type firewallRuleResourceModel struct {
	ID                  types.String   `tfsdk:"id"`
	IPAddress           types.String   `tfsdk:"ip_address"`
	Protocol            types.String   `tfsdk:"protocol"`
	CIDRList            types.String   `tfsdk:"cidr_list"`
	DestinationCIDRList types.String   `tfsdk:"destination_cidr_list"`
	StartPort           types.String   `tfsdk:"start_port"`
	EndPort             types.String   `tfsdk:"end_port"`
	State               types.String   `tfsdk:"state"`
	Timeouts            timeouts.Value `tfsdk:"timeouts"`
}

func NewFirewallRuleResource() resource.Resource {
	return &firewallRuleResource{}
}

func (r *firewallRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_rule"
}

func (r *firewallRuleResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP firewall rule on a public IP address.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Firewall rule unique identifier.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"ip_address": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Parent IP address slug (e.g. `1036521143`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"protocol": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Protocol for the rule (e.g. `tcp`, `udp`, `icmp`, `all`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"cidr_list": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Comma-separated list of source CIDRs (e.g. `0.0.0.0/0`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"destination_cidr_list": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Comma-separated list of destination CIDRs.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"start_port": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Start of the port range (e.g. `80`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"end_port": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "End of the port range (e.g. `80`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current state of the firewall rule.",
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

func (r *firewallRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = firewall.NewService(pd.Client)
}

func (r *firewallRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model firewallRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_firewall_rule cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	createReq := firewall.CreateRequest{
		Protocol: model.Protocol.ValueString(),
	}
	if !model.CIDRList.IsNull() && !model.CIDRList.IsUnknown() {
		createReq.CIDRList = model.CIDRList.ValueString()
	}
	if !model.DestinationCIDRList.IsNull() && !model.DestinationCIDRList.IsUnknown() {
		createReq.DestinationCIDRList = model.DestinationCIDRList.ValueString()
	}
	if !model.StartPort.IsNull() && !model.StartPort.IsUnknown() {
		createReq.StartPort = model.StartPort.ValueString()
	}
	if !model.EndPort.IsNull() && !model.EndPort.IsUnknown() {
		createReq.EndPort = model.EndPort.ValueString()
	}

	rule, err := r.svc.Create(ctx, model.IPAddress.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create firewall rule", err.Error())
		return
	}

	model.ID = types.StringValue(rule.ID)
	if rule.State != "" {
		model.State = types.StringValue(rule.State)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *firewallRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model firewallRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_firewall_rule cannot be read: bearer_token is missing.")
		return
	}

	rules, err := r.svc.List(ctx, model.IPAddress.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read firewall rule", err.Error())
		return
	}

	id := model.ID.ValueString()
	for _, rule := range rules {
		if rule.ID == id {
			model.Protocol = types.StringValue(rule.Protocol)
			if rule.CIDRList != "" {
				model.CIDRList = types.StringValue(rule.CIDRList)
			}
			if rule.DestinationCIDRList != "" {
				model.DestinationCIDRList = types.StringValue(rule.DestinationCIDRList)
			}
			if v := rule.StartPort; v != nil {
				model.StartPort = types.StringValue(fmt.Sprintf("%v", v))
			}
			if v := rule.EndPort; v != nil {
				model.EndPort = types.StringValue(fmt.Sprintf("%v", v))
			}
			if rule.State != "" {
				model.State = types.StringValue(rule.State)
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *firewallRuleResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are ForceNew; Terraform never invokes this method.
}

func (r *firewallRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model firewallRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_firewall_rule cannot be deleted: bearer_token is missing.")
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
		resp.Diagnostics.AddError("Failed to delete firewall rule", err.Error())
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
		resp.Diagnostics.AddError("Firewall rule deletion did not complete", err.Error())
	}
}

func (r *firewallRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Expected format: "ip-slug/rule-id"
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected format \"<ip_address>/<rule_id>\", got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip_address"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
