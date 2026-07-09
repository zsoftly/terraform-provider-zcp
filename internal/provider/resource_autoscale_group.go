package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/autoscale"
)

var _ resource.Resource = &autoscaleGroupResource{}
var _ resource.ResourceWithConfigure = &autoscaleGroupResource{}
var _ resource.ResourceWithImportState = &autoscaleGroupResource{}

// autoscaleServiceIface is shared by zcp_autoscale_group, zcp_autoscale_policy,
// and zcp_autoscale_condition.
type autoscaleServiceIface interface {
	List(ctx context.Context, region, project string) ([]autoscale.AutoscaleGroup, error)
	Create(ctx context.Context, req autoscale.CreateRequest) (*autoscale.AutoscaleGroup, error)
	Delete(ctx context.Context, slug string) error
	ChangePlan(ctx context.Context, slug, plan string) (*autoscale.AutoscaleGroup, error)
	ChangeTemplate(ctx context.Context, slug, template string) (*autoscale.AutoscaleGroup, error)
	Enable(ctx context.Context, slug string) (*autoscale.AutoscaleGroup, error)
	Disable(ctx context.Context, slug string) (*autoscale.AutoscaleGroup, error)
	CreatePolicy(ctx context.Context, slug string, req autoscale.PolicyRequest) (*autoscale.Policy, error)
	UpdatePolicy(ctx context.Context, slug string, policyID int, req autoscale.PolicyRequest) (*autoscale.Policy, error)
	DeletePolicy(ctx context.Context, slug string, policyID int) error
	CreateCondition(ctx context.Context, slug string, req autoscale.ConditionRequest) (*autoscale.Condition, error)
	UpdateCondition(ctx context.Context, slug string, conditionID int, req autoscale.ConditionRequest) (*autoscale.Condition, error)
	DeleteCondition(ctx context.Context, slug string, conditionID int) error
}

type autoscaleGroupResource struct {
	svc            autoscaleServiceIface
	defaultProject string
}

type autoscaleGroupResourceModel struct {
	ID             types.String   `tfsdk:"id"`
	Name           types.String   `tfsdk:"name"`
	Plan           types.String   `tfsdk:"plan"`
	Template       types.String   `tfsdk:"template"`
	MinInstances   types.Int64    `tfsdk:"min_instances"`
	MaxInstances   types.Int64    `tfsdk:"max_instances"`
	CooldownPeriod types.Int64    `tfsdk:"cooldown_period"`
	Zone           types.String   `tfsdk:"zone"`
	Network        types.String   `tfsdk:"network"`
	Enabled        types.Bool     `tfsdk:"enabled"`
	CloudProvider  types.String   `tfsdk:"cloud_provider"`
	Region         types.String   `tfsdk:"region"`
	Project        types.String   `tfsdk:"project"`
	State          types.String   `tfsdk:"state"`
	CurrentCount   types.Int64    `tfsdk:"current_count"`
	Timeouts       timeouts.Value `tfsdk:"timeouts"`
}

func NewAutoscaleGroupResource() resource.Resource {
	return &autoscaleGroupResource{}
}

func (r *autoscaleGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_autoscale_group"
}

func (r *autoscaleGroupResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP autoscale group. `plan`, `template`, and `enabled` update in place; " +
			"add scaling rules with `zcp_autoscale_policy` (scale up) and `zcp_autoscale_condition` (scale down).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Autoscale group slug.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name for the group. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"plan": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Compute plan slug for scaled instances. Updated in place.",
			},
			"template": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Template slug for scaled instances. Updated in place.",
			},
			"min_instances": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Minimum instance count. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"max_instances": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Maximum instance count. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"cooldown_period": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Cooldown period in seconds between scaling actions. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"zone": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Zone slug the group scales in. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"network": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Network slug scaled instances join. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether autoscaling is active. Toggled in place via the enable/disable API.",
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
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current state of the group.",
			},
			"current_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Current number of instances in the group.",
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

func (r *autoscaleGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = autoscale.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

func (r *autoscaleGroupResource) projectOrDefault(project types.String) string {
	if !project.IsNull() && !project.IsUnknown() {
		return project.ValueString()
	}
	return r.defaultProject
}

// applyGroupState populates the refreshable attributes from an AutoscaleGroup.
func applyGroupState(model *autoscaleGroupResourceModel, g *autoscale.AutoscaleGroup) {
	model.ID = types.StringValue(g.Slug)
	model.Name = types.StringValue(g.Name)
	model.State = types.StringValue(g.State)
	model.CurrentCount = types.Int64Value(int64(g.CurrentCount))
	if g.Plan != "" {
		model.Plan = types.StringValue(g.Plan)
	}
	if g.Template != "" {
		model.Template = types.StringValue(g.Template)
	}
	if g.MinInstances != 0 || g.MaxInstances != 0 {
		model.MinInstances = types.Int64Value(int64(g.MinInstances))
		model.MaxInstances = types.Int64Value(int64(g.MaxInstances))
	}
	if g.ZoneSlug != "" {
		model.Zone = types.StringValue(g.ZoneSlug)
	}
	if g.NetworkSlug != "" {
		model.Network = types.StringValue(g.NetworkSlug)
	}
}

func (r *autoscaleGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model autoscaleGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_autoscale_group cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 15*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	createReq := autoscale.CreateRequest{
		Name:          model.Name.ValueString(),
		Plan:          model.Plan.ValueString(),
		Template:      model.Template.ValueString(),
		MinInstances:  int(model.MinInstances.ValueInt64()),
		MaxInstances:  int(model.MaxInstances.ValueInt64()),
		ZoneSlug:      model.Zone.ValueString(),
		CloudProvider: model.CloudProvider.ValueString(),
		Region:        model.Region.ValueString(),
		Project:       r.projectOrDefault(model.Project),
	}
	if !model.CooldownPeriod.IsNull() && !model.CooldownPeriod.IsUnknown() {
		createReq.CooldownPeriod = int(model.CooldownPeriod.ValueInt64())
	}
	if !model.Network.IsNull() && !model.Network.IsUnknown() {
		createReq.NetworkSlug = model.Network.ValueString()
	}

	group, err := r.svc.Create(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create autoscale group", err.Error())
		return
	}
	if group == nil || group.Slug == "" {
		resp.Diagnostics.AddError("Failed to create autoscale group",
			fmt.Sprintf("the API accepted the create for %q but returned no group; check the autoscale group list before retrying.", model.Name.ValueString()))
		return
	}

	// An explicit enabled=false disables the group right after create.
	if !model.Enabled.IsNull() && !model.Enabled.IsUnknown() && !model.Enabled.ValueBool() {
		if disabled, derr := r.svc.Disable(ctx, group.Slug); derr != nil {
			// The group exists but is still enabled, which contradicts the
			// plan. Persist its real state and error so Terraform marks it
			// tainted and the next apply retries instead of recording a
			// disabled group that is actually running.
			model.Enabled = types.BoolValue(true)
			applyGroupState(&model, group)
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			resp.Diagnostics.AddError("Autoscale group created but not disabled",
				fmt.Sprintf("group %s was created; the requested disable failed: %s", group.Slug, derr))
			return
		} else if disabled != nil && disabled.Slug != "" {
			group = disabled
		}
	}

	applyGroupState(&model, group)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *autoscaleGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model autoscaleGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_autoscale_group cannot be read: bearer_token is missing.")
		return
	}

	groups, err := r.svc.List(ctx, model.Region.ValueString(), r.projectOrDefault(model.Project))
	if err != nil {
		resp.Diagnostics.AddError("Failed to read autoscale group", err.Error())
		return
	}

	slug := model.ID.ValueString()
	for i := range groups {
		if groups[i].Slug == slug {
			applyGroupState(&model, &groups[i])
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *autoscaleGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state autoscaleGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_autoscale_group cannot be updated: bearer_token is missing.")
		return
	}

	updateTimeout, diags := plan.Timeouts.Update(ctx, 15*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	slug := state.ID.ValueString()
	model := plan
	model.ID = state.ID
	model.State = state.State
	model.CurrentCount = state.CurrentCount

	// fresh carries the latest group returned by a lifecycle call, so state
	// and current_count are written from live data instead of the old state.
	var fresh *autoscale.AutoscaleGroup

	if plan.Plan.ValueString() != state.Plan.ValueString() {
		g, err := r.svc.ChangePlan(ctx, slug, plan.Plan.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Failed to change autoscale group plan", err.Error())
			return
		}
		if g != nil && g.Slug != "" {
			fresh = g
		}
	}
	if plan.Template.ValueString() != state.Template.ValueString() {
		g, err := r.svc.ChangeTemplate(ctx, slug, plan.Template.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Failed to change autoscale group template", err.Error())
			return
		}
		if g != nil && g.Slug != "" {
			fresh = g
		}
	}

	planEnabled := plan.Enabled.IsNull() || plan.Enabled.IsUnknown() || plan.Enabled.ValueBool()
	stateEnabled := state.Enabled.IsNull() || state.Enabled.ValueBool()
	if planEnabled != stateEnabled {
		var g *autoscale.AutoscaleGroup
		var err error
		if planEnabled {
			g, err = r.svc.Enable(ctx, slug)
		} else {
			g, err = r.svc.Disable(ctx, slug)
		}
		if err != nil {
			resp.Diagnostics.AddError("Failed to toggle autoscale group", err.Error())
			return
		}
		if g != nil && g.Slug != "" {
			fresh = g
		}
	}

	if fresh != nil {
		applyGroupState(&model, fresh)
		model.ID = state.ID
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *autoscaleGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model autoscaleGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_autoscale_group cannot be deleted: bearer_token is missing.")
		return
	}

	deleteTimeout, diags := model.Timeouts.Delete(ctx, 15*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	slug := model.ID.ValueString()
	err := r.svc.Delete(deleteCtx, slug)
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete autoscale group", err.Error())
		return
	}

	region := model.Region.ValueString()
	project := r.projectOrDefault(model.Project)
	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		groups, err := r.svc.List(ctx, region, project)
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for i := range groups {
			if groups[i].Slug == slug {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError("Autoscale group deletion did not complete", err.Error())
	}
}

// ImportState accepts a composite ID so the write-only scope fields are seeded
// for a zero-diff plan after import. Format:
//
//	<slug>/<region>/<cloud_provider>[/<project>]
//
// plan, template, instance bounds, zone, and network come from the subsequent Read.
func (r *autoscaleGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"id", "region", "cloud_provider", "project"}
	importPositional(ctx, req, resp, fields, 3, "<slug>/<region>/<cloud_provider>[/<project>]")
}
