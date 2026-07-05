package provider

import (
	"context"
	"encoding/json"
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
	"github.com/zsoftly/zcp-cli/pkg/api/volume"
)

var _ resource.Resource = &volumeResource{}
var _ resource.ResourceWithConfigure = &volumeResource{}
var _ resource.ResourceWithImportState = &volumeResource{}
var _ resource.ResourceWithValidateConfig = &volumeResource{}

type volumeServiceIface interface {
	Create(ctx context.Context, req volume.CreateRequest) (*volume.Volume, error)
	List(ctx context.Context, region, project string) ([]volume.Volume, error)
	Attach(ctx context.Context, volumeSlug, vmSlug string) (*volume.Volume, error)
	Detach(ctx context.Context, volumeSlug string) (*volume.Volume, error)
	Delete(ctx context.Context, slug string) error
}

type volumeResource struct {
	svc            volumeServiceIface
	defaultProject string
}

type volumeResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	CloudProvider   types.String `tfsdk:"cloud_provider"`
	Region          types.String `tfsdk:"region"`
	BillingCycle    types.String `tfsdk:"billing_cycle"`
	StorageCategory types.String `tfsdk:"storage_category"`
	Plan            types.String `tfsdk:"plan"`
	Size            types.Int64  `tfsdk:"size"`
	Project         types.String `tfsdk:"project"`
	VM              types.String `tfsdk:"vm"`
	// Computed
	Slug     types.String   `tfsdk:"slug"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func NewVolumeResource() resource.Resource {
	return &volumeResource{}
}

// volumeSizeValue resolves the Optional+Computed size attribute: it prefers the
// size the API reports (a json.Number), falling back to the current plan/state
// value, and finally to 0 so no unknown is ever left after apply.
func volumeSizeValue(current types.Int64, apiSize json.Number) types.Int64 {
	if n, err := apiSize.Int64(); err == nil && n > 0 {
		return types.Int64Value(n)
	}
	if !current.IsNull() && !current.IsUnknown() {
		return current
	}
	return types.Int64Value(0)
}

func (r *volumeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_volume"
}

func (r *volumeResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP block storage volume. Provide exactly one of `plan` or `size`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Volume slug (unique identifier).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name for the volume. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"cloud_provider": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cloud provider slug. Use `data.zcp_region.<name>.cloud_provider` instead of hardcoding. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Region slug where the volume is created (e.g. `yow-1`). Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"billing_cycle": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Billing cycle (`hourly` or `monthly`). Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"storage_category": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Storage category slug. Region-specific: `nvme`/`hdd-storage` in yow-1, `pro-nvme`/`premium-ssd` in yul-1. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"plan": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Plan slug (e.g. `b1g1`). Mutually exclusive with `size`. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"size": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Storage size in GB. Set this for a custom-size volume (mutually exclusive with `plan`); for a plan-based volume it is computed from the plan. Changing it forces replacement.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace(), int64planmodifier.UseStateForUnknown()},
			},
			"project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project slug. Inherits from the provider `default_project` if omitted. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"vm": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Virtual machine slug to attach the volume to. Set, change, or clear this to attach/detach the volume in place.",
			},
			"slug": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Volume slug (same value as `id`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
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

// ValidateConfig enforces the "exactly one of plan or size" rule at plan time
// (the API has no single endpoint that accepts both, and neither is meaningful).
func (r *volumeResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var model volumeResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Skip when either is unknown (e.g. a reference to a not-yet-created value):
	// it may resolve at apply time.
	if model.Plan.IsUnknown() || model.Size.IsUnknown() {
		return
	}
	planSet := !model.Plan.IsNull()
	sizeSet := !model.Size.IsNull()
	switch {
	case planSet && sizeSet:
		resp.Diagnostics.AddError(
			"Conflicting volume sizing",
			"`plan` and `size` are mutually exclusive — set exactly one.",
		)
	case !planSet && !sizeSet:
		resp.Diagnostics.AddError(
			"Missing volume sizing",
			"One of `plan` or `size` must be set.",
		)
	}
}

func (r *volumeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = volume.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

func (r *volumeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model volumeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_volume cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 15*time.Minute)
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

	createReq := volume.CreateRequest{
		Name:            model.Name.ValueString(),
		Project:         project,
		CloudProvider:   model.CloudProvider.ValueString(),
		Region:          model.Region.ValueString(),
		BillingCycle:    model.BillingCycle.ValueString(),
		StorageCategory: model.StorageCategory.ValueString(),
		Plan:            model.Plan.ValueString(),
		VirtualMachine:  model.VM.ValueString(),
	}
	if !model.Size.IsNull() && !model.Size.IsUnknown() {
		createReq.IsCustomPlan = true
		createReq.CustomPlan = &volume.CustomPlanStorage{Storage: int(model.Size.ValueInt64())}
	}

	vol, err := r.svc.Create(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create volume", err.Error())
		return
	}

	model.ID = types.StringValue(vol.Slug)
	model.Slug = types.StringValue(vol.Slug)
	// size is Optional+Computed: always resolve it to a known value (the API
	// echoes the actual size for both custom and plan-based volumes) so no
	// unknown remains after apply.
	model.Size = volumeSizeValue(model.Size, vol.Size)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *volumeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model volumeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_volume cannot be read: bearer_token is missing.")
		return
	}

	project := r.defaultProject
	if !model.Project.IsNull() && !model.Project.IsUnknown() {
		project = model.Project.ValueString()
	}
	// The volume API has no single-object GET; list within the region/project
	// and match by slug.
	volumes, err := r.svc.List(ctx, model.Region.ValueString(), project)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read volume", err.Error())
		return
	}

	slug := model.ID.ValueString()
	for _, v := range volumes {
		if v.Slug == slug {
			model.Name = types.StringValue(v.Name)
			model.Slug = types.StringValue(v.Slug)
			model.Size = volumeSizeValue(model.Size, v.Size)
			// vm is not refreshed from the list response (it returns an internal
			// VM id, not the slug the user configured); preserved from state.
			// plan, size, cloud_provider, billing_cycle, storage_category and
			// project are create-only and likewise preserved from state.
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

// Update handles the only mutable attribute, `vm`: attaching, detaching, or
// re-attaching the volume in place when it changes.
func (r *volumeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model volumeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state volumeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_volume cannot be updated: bearer_token is missing.")
		return
	}

	updateTimeout, diags := model.Timeouts.Update(ctx, 15*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	slug := state.ID.ValueString()
	oldVM := state.VM.ValueString()
	newVM := model.VM.ValueString()

	if oldVM != newVM {
		// Detach from the previous VM first (if any), then attach to the new one.
		if oldVM != "" {
			if _, err := r.svc.Detach(ctx, slug); err != nil && !apierrors.IsNotFound(err) {
				resp.Diagnostics.AddError("Failed to detach volume", err.Error())
				return
			}
		}
		if newVM != "" {
			if _, err := r.svc.Attach(ctx, slug, newVM); err != nil {
				resp.Diagnostics.AddError("Failed to attach volume", err.Error())
				return
			}
		}
	}

	model.ID = state.ID
	model.Slug = state.Slug
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *volumeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model volumeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_volume cannot be deleted: bearer_token is missing.")
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
	// A volume must be detached before it can be deleted. Detach proactively when
	// state says it is attached.
	if model.VM.ValueString() != "" {
		if _, err := r.svc.Detach(deleteCtx, slug); err != nil && !apierrors.IsNotFound(err) {
			resp.Diagnostics.AddError("Failed to detach volume before delete", err.Error())
			return
		}
	}

	err := r.svc.Delete(deleteCtx, slug)
	if err != nil && !apierrors.IsNotFound(err) && !apierrors.IsResourceNotFound(err) {
		// The first delete may fail because the volume is attached out-of-band
		// (state.vm is stale or empty). Detach, then retry the delete once. The
		// API derives the VM from the volume, so no VM slug is needed.
		if _, derr := r.svc.Detach(deleteCtx, slug); derr == nil || apierrors.IsNotFound(derr) {
			err = r.svc.Delete(deleteCtx, slug)
		}
		if err != nil && !apierrors.IsNotFound(err) && !apierrors.IsResourceNotFound(err) {
			resp.Diagnostics.AddError("Failed to delete volume", err.Error())
			return
		}
	}
}

// ImportState seeds the create-only attributes the list response does not return
// in a comparable form so a post-import plan is zero-diff. Format
// (slash-separated, trailing/empty segments allowed):
//
//	<slug>/<cloud_provider>/<region>/<billing_cycle>/<storage_category>/<plan>/<project>/<vm>
//
// <slug>/<cloud_provider>/<region>/<billing_cycle>/<storage_category> are
// required. `size` cannot be imported via the ID (it is an int); set it in
// config for size-based volumes. name and slug come from the subsequent Read.
func (r *volumeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"id", "cloud_provider", "region", "billing_cycle", "storage_category", "plan", "project", "vm"}
	importPositional(ctx, req, resp, fields, 5,
		"<slug>/<cloud_provider>/<region>/<billing_cycle>/<storage_category>[/<plan>/<project>/<vm>]")
}
