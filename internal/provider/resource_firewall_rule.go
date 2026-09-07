package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/firewall"
	"github.com/zsoftly/zcp-cli/pkg/api/ipaddress"
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
	svc            firewallServiceIface
	ipSvc          publicIPLister
	defaultProject string
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
		MarkdownDescription: "Manages a ZCP firewall rule on a public IP address. Firewall rules do not apply to a public IP that belongs to a VPC. Use `zcp_network_acl` and `zcp_network_acl_rule` to control ingress for a VPC tier instead.",
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
	r.ipSvc = ipaddress.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

// checkNotVPCPublicIP looks up ipSlug in the account IP list and appends an
// error diagnostic if it belongs to a VPC: the API accepts a firewall rule
// create request on a VPC public IP but never applies it, so the create would
// otherwise poll until timeout instead of failing. The check is best effort: a
// lister failure or a slug that is not found in the list does not block the
// create, since the create call's own error handling already covers a
// genuinely invalid IP.
func (r *firewallRuleResource) checkNotVPCPublicIP(ctx context.Context, ipSlug string, diags *diag.Diagnostics) {
	if r.ipSvc == nil {
		return
	}
	// The resource has no region/project attributes of its own, so the list
	// call stays unscoped by region. It is scoped to the provider's default
	// project when one is configured, and falls back to unscoped otherwise.
	ips, err := r.ipSvc.List(ctx, "", "", r.defaultProject)
	if err != nil {
		tflog.Warn(ctx, "could not check whether the IP address belongs to a VPC, continuing", map[string]interface{}{
			"ip_address": ipSlug,
			"error":      err.Error(),
		})
		return
	}
	for _, ip := range ips {
		if ip.Slug != ipSlug {
			continue
		}
		if ip.VPCID != "" {
			diags.AddError(
				"Firewall rules are not supported on VPC public IPs",
				fmt.Sprintf(
					"IP %s belongs to a VPC. The API accepts a zcp_firewall_rule create request on a VPC public IP but never applies it, so the rule would poll until timeout instead of failing. Control ingress for a VPC tier with zcp_network_acl and zcp_network_acl_rule instead.",
					ipSlug,
				),
			)
		}
		return
	}
	tflog.Warn(ctx, "IP address not found in account IP list, continuing", map[string]interface{}{
		"ip_address": ipSlug,
	})
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

	ipSlug := model.IPAddress.ValueString()
	r.checkNotVPCPublicIP(ctx, ipSlug, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.svc.Create(ctx, ipSlug, createReq); err != nil {
		resp.Diagnostics.AddError("Failed to create firewall rule", err.Error())
		return
	}

	// Creation is asynchronous and returns no rule object (data: null), so poll
	// the rule list and match on protocol, ports, and CIDR to recover the new
	// rule's ID and state.
	var found firewall.FirewallRule
	if err := pollUntilReady(ctx, 5*time.Second, func(ctx context.Context) (bool, error) {
		rules, err := r.svc.List(ctx, ipSlug)
		if err != nil {
			return false, err
		}
		for _, rule := range rules {
			if firewallRuleMatches(rule, model) {
				found = rule
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError(
			"Firewall rule did not appear after create",
			fmt.Sprintf("the rule was accepted but never showed up on IP %s: %s", ipSlug, err),
		)
		return
	}

	// The match keys on protocol and ports, not on the ID, so guard against a
	// matched rule that came back without one. Persisting an empty ID would put
	// the resource right back into the recreate loop this fix removes.
	if found.ID == "" {
		resp.Diagnostics.AddError(
			"Firewall rule created without an ID",
			fmt.Sprintf("the matching rule on IP %s was returned without an ID, so it cannot be tracked. Check the rule manually and remove it if it is orphaned.", ipSlug),
		)
		return
	}

	model.ID = types.StringValue(found.ID)
	// state is Computed; an unknown value after Create fails the apply.
	model.State = stateOrNull(found.State)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// firewallRuleMatches reports whether a listed rule is the one just created.
// Creation returns no ID, so the rule is identified by its fields. The match is
// built to avoid false negatives, which are the dangerous case: a rule that was
// created but not matched fails the apply and is left orphaned. So a plan field
// that is blank (an icmp rule with no ports, an omitted end port or CIDR) does
// not exclude a rule, ports compare with 0 treated as no-port, and CIDR lists
// compare unordered.
//
// Known limitation: if an identical rule already exists on the IP (or the
// platform adds a same-port companion), the first list match wins, so the wrong
// rule's ID can be recorded. This is inherent to the API returning no
// correlation token on create.
func firewallRuleMatches(rule firewall.FirewallRule, model firewallRuleResourceModel) bool {
	if !strings.EqualFold(rule.Protocol, model.Protocol.ValueString()) {
		return false
	}
	if !fwPortEqual(fwPortString(rule.StartPort), model.StartPort.ValueString()) {
		return false
	}
	// Only narrow on the end port when the plan set one: a single-port rule may
	// come back with the end port equal to the start or absent, and either must
	// still match.
	if model.EndPort.ValueString() != "" && !fwPortEqual(fwPortString(rule.EndPort), model.EndPort.ValueString()) {
		return false
	}
	if model.CIDRList.ValueString() != "" && !cidrListEqual(rule.CIDRList, model.CIDRList.ValueString()) {
		return false
	}
	if model.DestinationCIDRList.ValueString() != "" && !cidrListEqual(rule.DestinationCIDRList, model.DestinationCIDRList.ValueString()) {
		return false
	}
	return true
}

// fwPortString renders a firewall rule port (returned as a string or a JSON
// number) for comparison. A nil port becomes the empty string.
func fwPortString(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// fwPortEqual compares two firewall port strings, treating "0" and "" as the
// same absent value so an icmp rule matches whether the API returns 0 or null.
func fwPortEqual(a, b string) bool {
	norm := func(s string) string {
		if s == "0" {
			return ""
		}
		return s
	}
	return norm(a) == norm(b)
}

// cidrListEqual compares two comma-separated CIDR lists as unordered sets. The
// API may reorder or re-space the value it echoes, so byte equality is unsafe.
func cidrListEqual(a, b string) bool {
	return equalStringSet(splitCIDRs(a), splitCIDRs(b))
}

func splitCIDRs(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func equalStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, x := range a {
		seen[x]++
	}
	for _, x := range b {
		seen[x]--
		if seen[x] < 0 {
			return false
		}
	}
	return true
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
	err := r.svc.Delete(deleteCtx, ipSlug, ruleID)
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
