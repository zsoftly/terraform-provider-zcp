package provider

import (
	"context"
	"fmt"
	"strconv"
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
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/autoscale"
)

// autoscaleRule is the common shape of scale-up policies and scale-down
// conditions; the API models them as separate endpoints with identical fields.
type autoscaleRule struct {
	ID          string
	Name        string
	Metric      string
	Operator    string
	Threshold   int
	Duration    int
	ScaleAmount int
	Cooldown    int
}

// autoscaleRuleKind wires one of the two rule endpoints into the shared
// resource implementation.
type autoscaleRuleKind struct {
	typeSuffix  string // resource type suffix, e.g. "autoscale_policy"
	displayName string // used in error messages, e.g. "autoscale policy"
	description string
	rulesOf     func(g *autoscale.AutoscaleGroup) []autoscaleRule
	create      func(ctx context.Context, svc autoscaleServiceIface, groupSlug string, rule autoscaleRule) (string, error)
	update      func(ctx context.Context, svc autoscaleServiceIface, groupSlug string, id int, rule autoscaleRule) error
	remove      func(ctx context.Context, svc autoscaleServiceIface, groupSlug string, id int) error
}

func policiesOf(g *autoscale.AutoscaleGroup) []autoscaleRule {
	rules := make([]autoscaleRule, 0, len(g.Policies))
	for _, p := range g.Policies {
		rules = append(rules, autoscaleRule{ID: p.ID, Name: p.Name, Metric: p.Metric, Operator: p.Operator,
			Threshold: p.Threshold, Duration: p.Duration, ScaleAmount: p.ScaleAmount, Cooldown: p.Cooldown})
	}
	return rules
}

func conditionsOf(g *autoscale.AutoscaleGroup) []autoscaleRule {
	rules := make([]autoscaleRule, 0, len(g.Conditions))
	for _, c := range g.Conditions {
		rules = append(rules, autoscaleRule{ID: c.ID, Name: c.Name, Metric: c.Metric, Operator: c.Operator,
			Threshold: c.Threshold, Duration: c.Duration, ScaleAmount: c.ScaleAmount, Cooldown: c.Cooldown})
	}
	return rules
}

func policyRequest(rule autoscaleRule) autoscale.PolicyRequest {
	return autoscale.PolicyRequest{Name: rule.Name, Metric: rule.Metric, Operator: rule.Operator,
		Threshold: rule.Threshold, Duration: rule.Duration, ScaleAmount: rule.ScaleAmount, Cooldown: rule.Cooldown}
}

func conditionRequest(rule autoscaleRule) autoscale.ConditionRequest {
	return autoscale.ConditionRequest{Name: rule.Name, Metric: rule.Metric, Operator: rule.Operator,
		Threshold: rule.Threshold, Duration: rule.Duration, ScaleAmount: rule.ScaleAmount, Cooldown: rule.Cooldown}
}

var autoscalePolicyKind = autoscaleRuleKind{
	typeSuffix:  "autoscale_policy",
	displayName: "autoscale policy",
	description: "Manages a scale-up policy on a `zcp_autoscale_group`. All rule fields update in place; changing `autoscale_group` forces replacement.",
	rulesOf:     policiesOf,
	create: func(ctx context.Context, svc autoscaleServiceIface, groupSlug string, rule autoscaleRule) (string, error) {
		created, err := svc.CreatePolicy(ctx, groupSlug, policyRequest(rule))
		if err != nil {
			return "", err
		}
		return created.ID, nil
	},
	update: func(ctx context.Context, svc autoscaleServiceIface, groupSlug string, id int, rule autoscaleRule) error {
		_, err := svc.UpdatePolicy(ctx, groupSlug, id, policyRequest(rule))
		return err
	},
	remove: func(ctx context.Context, svc autoscaleServiceIface, groupSlug string, id int) error {
		return svc.DeletePolicy(ctx, groupSlug, id)
	},
}

var autoscaleConditionKind = autoscaleRuleKind{
	typeSuffix:  "autoscale_condition",
	displayName: "autoscale condition",
	description: "Manages a scale-down condition on a `zcp_autoscale_group`. All rule fields update in place; changing `autoscale_group` forces replacement.",
	rulesOf:     conditionsOf,
	create: func(ctx context.Context, svc autoscaleServiceIface, groupSlug string, rule autoscaleRule) (string, error) {
		created, err := svc.CreateCondition(ctx, groupSlug, conditionRequest(rule))
		if err != nil {
			return "", err
		}
		return created.ID, nil
	},
	update: func(ctx context.Context, svc autoscaleServiceIface, groupSlug string, id int, rule autoscaleRule) error {
		_, err := svc.UpdateCondition(ctx, groupSlug, id, conditionRequest(rule))
		return err
	},
	remove: func(ctx context.Context, svc autoscaleServiceIface, groupSlug string, id int) error {
		return svc.DeleteCondition(ctx, groupSlug, id)
	},
}

var _ resource.Resource = &autoscaleRuleResource{}
var _ resource.ResourceWithConfigure = &autoscaleRuleResource{}
var _ resource.ResourceWithImportState = &autoscaleRuleResource{}

type autoscaleRuleResource struct {
	svc  autoscaleServiceIface
	kind autoscaleRuleKind
}

type autoscaleRuleResourceModel struct {
	ID             types.String   `tfsdk:"id"`
	AutoscaleGroup types.String   `tfsdk:"autoscale_group"`
	Name           types.String   `tfsdk:"name"`
	Metric         types.String   `tfsdk:"metric"`
	Operator       types.String   `tfsdk:"operator"`
	Threshold      types.Int64    `tfsdk:"threshold"`
	Duration       types.Int64    `tfsdk:"duration"`
	ScaleAmount    types.Int64    `tfsdk:"scale_amount"`
	Cooldown       types.Int64    `tfsdk:"cooldown"`
	Timeouts       timeouts.Value `tfsdk:"timeouts"`
}

func NewAutoscalePolicyResource() resource.Resource {
	return &autoscaleRuleResource{kind: autoscalePolicyKind}
}

func NewAutoscaleConditionResource() resource.Resource {
	return &autoscaleRuleResource{kind: autoscaleConditionKind}
}

func (r *autoscaleRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.kind.typeSuffix
}

func (r *autoscaleRuleResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: r.kind.description,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Rule ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"autoscale_group": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Parent autoscale group slug. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Rule name. Updated in place.",
			},
			"metric": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Metric the rule evaluates (e.g. `cpu`, `memory`). Updated in place.",
			},
			"operator": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Comparison operator (e.g. `GT`, `LT`). Updated in place.",
			},
			"threshold": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Metric threshold triggering the rule. Updated in place.",
			},
			"duration": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Seconds the metric must breach the threshold before scaling. Updated in place.",
			},
			"scale_amount": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Number of instances to add or remove per scaling action. Updated in place.",
			},
			"cooldown": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Cooldown in seconds after this rule fires. Updated in place.",
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

func (r *autoscaleRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = autoscale.NewService(pd.Client)
}

func (r *autoscaleRuleResource) ruleFromModel(model *autoscaleRuleResourceModel) autoscaleRule {
	rule := autoscaleRule{
		Name:        model.Name.ValueString(),
		Metric:      model.Metric.ValueString(),
		Operator:    model.Operator.ValueString(),
		Threshold:   int(model.Threshold.ValueInt64()),
		Duration:    int(model.Duration.ValueInt64()),
		ScaleAmount: int(model.ScaleAmount.ValueInt64()),
	}
	if !model.Cooldown.IsNull() && !model.Cooldown.IsUnknown() {
		rule.Cooldown = int(model.Cooldown.ValueInt64())
	}
	return rule
}

func (r *autoscaleRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model autoscaleRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", fmt.Sprintf("zcp_%s cannot be created: bearer_token is missing.", r.kind.typeSuffix))
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	groupSlug := model.AutoscaleGroup.ValueString()
	rule := r.ruleFromModel(&model)
	id, err := r.kind.create(ctx, r.svc, groupSlug, rule)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Failed to create %s", r.kind.displayName), err.Error())
		return
	}
	if id == "" {
		// The create response omitted the rule; resolve it from the group by
		// matching all writable fields.
		if found := r.findRule(ctx, groupSlug, func(candidate autoscaleRule) bool {
			return candidate.Name == rule.Name && candidate.Metric == rule.Metric &&
				candidate.Operator == rule.Operator && candidate.Threshold == rule.Threshold
		}); found != nil {
			id = found.ID
		}
	}
	if id == "" {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to resolve created %s", r.kind.displayName),
			fmt.Sprintf("rule %q was created on %q but its ID is unknown.", rule.Name, groupSlug),
		)
		return
	}

	model.ID = types.StringValue(id)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// findRule scans the group's rules for the first one matching the predicate.
func (r *autoscaleRuleResource) findRule(ctx context.Context, groupSlug string, match func(autoscaleRule) bool) *autoscaleRule {
	groups, err := r.svc.List(ctx, "", "")
	if err != nil {
		return nil
	}
	for i := range groups {
		if groups[i].Slug != groupSlug {
			continue
		}
		for _, rule := range r.kind.rulesOf(&groups[i]) {
			if match(rule) {
				found := rule
				return &found
			}
		}
	}
	return nil
}

func (r *autoscaleRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model autoscaleRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", fmt.Sprintf("zcp_%s cannot be read: bearer_token is missing.", r.kind.typeSuffix))
		return
	}

	id := model.ID.ValueString()
	found := r.findRuleWithErr(ctx, model.AutoscaleGroup.ValueString(), id, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	model.Name = types.StringValue(found.Name)
	model.Metric = types.StringValue(found.Metric)
	model.Operator = types.StringValue(found.Operator)
	model.Threshold = types.Int64Value(int64(found.Threshold))
	model.Duration = types.Int64Value(int64(found.Duration))
	model.ScaleAmount = types.Int64Value(int64(found.ScaleAmount))
	if found.Cooldown != 0 || !model.Cooldown.IsNull() {
		model.Cooldown = types.Int64Value(int64(found.Cooldown))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// findRuleWithErr locates a rule by ID, surfacing list errors via diags.
func (r *autoscaleRuleResource) findRuleWithErr(ctx context.Context, groupSlug, id string, diags *diag.Diagnostics) *autoscaleRule {
	groups, err := r.svc.List(ctx, "", "")
	if err != nil {
		diags.AddError(fmt.Sprintf("Failed to read %s", r.kind.displayName), err.Error())
		return nil
	}
	for i := range groups {
		if groups[i].Slug != groupSlug {
			continue
		}
		for _, rule := range r.kind.rulesOf(&groups[i]) {
			if rule.ID == id {
				found := rule
				return &found
			}
		}
	}
	return nil
}

func (r *autoscaleRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state autoscaleRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", fmt.Sprintf("zcp_%s cannot be updated: bearer_token is missing.", r.kind.typeSuffix))
		return
	}

	updateTimeout, diags := plan.Timeouts.Update(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	id, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid rule ID", fmt.Sprintf("rule ID %q is not numeric: %s", state.ID.ValueString(), err))
		return
	}
	if err := r.kind.update(ctx, r.svc, state.AutoscaleGroup.ValueString(), id, r.ruleFromModel(&plan)); err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Failed to update %s", r.kind.displayName), err.Error())
		return
	}

	model := plan
	model.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *autoscaleRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model autoscaleRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", fmt.Sprintf("zcp_%s cannot be deleted: bearer_token is missing.", r.kind.typeSuffix))
		return
	}

	deleteTimeout, diags := model.Timeouts.Delete(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	id, err := strconv.Atoi(model.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid rule ID", fmt.Sprintf("rule ID %q is not numeric: %s", model.ID.ValueString(), err))
		return
	}
	if err := r.kind.remove(deleteCtx, r.svc, model.AutoscaleGroup.ValueString(), id); err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError(fmt.Sprintf("Failed to delete %s", r.kind.displayName), err.Error())
	}
}

func (r *autoscaleRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Expected format: "group-slug/rule-id"
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected format \"<autoscale_group>/<rule_id>\", got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("autoscale_group"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
