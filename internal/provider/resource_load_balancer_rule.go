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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/loadbalancer"
)

var _ resource.Resource = &loadBalancerRuleResource{}
var _ resource.ResourceWithConfigure = &loadBalancerRuleResource{}
var _ resource.ResourceWithImportState = &loadBalancerRuleResource{}

type loadBalancerRuleResource struct {
	svc loadBalancerServiceIface
}

type loadBalancerRuleResourceModel struct {
	ID           types.String   `tfsdk:"id"`
	LoadBalancer types.String   `tfsdk:"load_balancer"`
	Name         types.String   `tfsdk:"name"`
	PublicPort   types.String   `tfsdk:"public_port"`
	PrivatePort  types.String   `tfsdk:"private_port"`
	Protocol     types.String   `tfsdk:"protocol"`
	Algorithm    types.String   `tfsdk:"algorithm"`
	StickyMethod types.String   `tfsdk:"sticky_method"`
	EnableTLS    types.Bool     `tfsdk:"enable_tls"`
	EnableProxy  types.Bool     `tfsdk:"enable_proxy"`
	Timeouts     timeouts.Value `tfsdk:"timeouts"`
}

func NewLoadBalancerRuleResource() resource.Resource {
	return &loadBalancerRuleResource{}
}

func (r *loadBalancerRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_load_balancer_rule"
}

func (r *loadBalancerRuleResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an additional rule on a `zcp_load_balancer`. The API has no update endpoint for rules, so every change forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Rule ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"load_balancer": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Parent load balancer slug. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Rule name, unique within the load balancer. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"public_port": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Public port (e.g. `80`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"private_port": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Private port (e.g. `8080`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"protocol": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Protocol (e.g. `tcp`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"algorithm": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Balancing algorithm: `roundrobin`, `leastconn`, or `source`. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"sticky_method": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Session stickiness method (e.g. `LbCookie`, `SourceBased`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enable_tls": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable TLS on the rule. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"enable_proxy": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable the PROXY protocol on the rule. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
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

func (r *loadBalancerRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = &loadBalancerService{Service: loadbalancer.NewService(pd.Client), client: pd.Client}
}

// findLBBySlug scans the account-wide list for the given load balancer.
func findLBBySlug(lbs []loadbalancer.LoadBalancer, slug string) *loadbalancer.LoadBalancer {
	for i := range lbs {
		if lbs[i].Slug == slug {
			return &lbs[i]
		}
	}
	return nil
}

func (r *loadBalancerRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model loadBalancerRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_load_balancer_rule cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	lbSlug := model.LoadBalancer.ValueString()
	spec := buildCreateRuleSpec(
		model.Name.ValueString(),
		model.PublicPort.ValueString(), model.PrivatePort.ValueString(),
		model.Protocol.ValueString(), model.Algorithm.ValueString(),
		model.StickyMethod.ValueString(), model.EnableTLS, model.EnableProxy,
	)

	if err := r.svc.CreateRule(ctx, lbSlug, loadbalancer.CreateRuleRequest{Rules: []loadbalancer.CreateRuleSpec{spec}}); err != nil {
		resp.Diagnostics.AddError("Failed to create load balancer rule", err.Error())
		return
	}

	// The create endpoint returns no rule body; resolve the ID from the list by
	// name + ports.
	lbs, err := r.svc.List(ctx, "", "")
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve created load balancer rule", err.Error())
		return
	}
	lb := findLBBySlug(lbs, lbSlug)
	if lb == nil {
		resp.Diagnostics.AddError("Failed to resolve created load balancer rule", fmt.Sprintf("load balancer %q not found after rule create.", lbSlug))
		return
	}
	for _, rule := range lb.Rules {
		if rule.Name == spec.Name && rule.PublicPort == spec.PublicPort && rule.PrivatePort == spec.PrivatePort {
			model.ID = types.StringValue(rule.ID)
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.Diagnostics.AddError(
		"Failed to resolve created load balancer rule",
		fmt.Sprintf("rule %q was created on %q but does not appear in the rule list.", spec.Name, lbSlug),
	)
}

func (r *loadBalancerRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model loadBalancerRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_load_balancer_rule cannot be read: bearer_token is missing.")
		return
	}

	lbs, err := r.svc.List(ctx, "", "")
	if err != nil {
		resp.Diagnostics.AddError("Failed to read load balancer rule", err.Error())
		return
	}
	lb := findLBBySlug(lbs, model.LoadBalancer.ValueString())
	if lb == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	id := model.ID.ValueString()
	for _, rule := range lb.Rules {
		if rule.ID == id {
			model.Name = types.StringValue(rule.Name)
			model.PublicPort = types.StringValue(rule.PublicPort)
			model.PrivatePort = types.StringValue(rule.PrivatePort)
			if rule.Protocol != "" {
				model.Protocol = types.StringValue(rule.Protocol)
			}
			if rule.Algorithm != "" {
				model.Algorithm = types.StringValue(rule.Algorithm)
			}
			if rule.StickyMethod != "" {
				model.StickyMethod = types.StringValue(rule.StickyMethod)
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *loadBalancerRuleResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are ForceNew; Terraform never invokes this method.
}

func (r *loadBalancerRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model loadBalancerRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_load_balancer_rule cannot be deleted: bearer_token is missing.")
		return
	}

	deleteTimeout, diags := model.Timeouts.Delete(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	lbSlug := model.LoadBalancer.ValueString()
	ruleID := model.ID.ValueString()
	err := r.svc.DeleteRule(deleteCtx, lbSlug, ruleID)
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete load balancer rule", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		lbs, err := r.svc.List(ctx, "", "")
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		lb := findLBBySlug(lbs, lbSlug)
		if lb == nil {
			return false, nil
		}
		for _, rule := range lb.Rules {
			if rule.ID == ruleID {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError("Load balancer rule deletion did not complete", err.Error())
	}
}

func (r *loadBalancerRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Expected format: "lb-slug/rule-id"
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected format \"<load_balancer>/<rule_id>\", got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("load_balancer"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
