package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/backup"
)

// defaultVolumeBackupPseudoService matches the CLI's fallback for
// --pseudo-service on `zcp backup create`.
const defaultVolumeBackupPseudoService = "Virtual Machine Backup"

var _ resource.Resource = &volumeBackupResource{}
var _ resource.ResourceWithConfigure = &volumeBackupResource{}
var _ resource.ResourceWithImportState = &volumeBackupResource{}

type volumeBackupServiceIface interface {
	List(ctx context.Context, region, project string) ([]backup.Backup, error)
	Create(ctx context.Context, blockstorageSlug string, req backup.CreateRequest) (*backup.Backup, error)
	Delete(ctx context.Context, slug string) error
}

type volumeBackupResource struct {
	svc            volumeBackupServiceIface
	defaultProject string
}

type volumeBackupResourceModel struct {
	ID            types.String   `tfsdk:"id"`
	Volume        types.String   `tfsdk:"volume"`
	Interval      types.String   `tfsdk:"interval"`
	At            types.Int64    `tfsdk:"at"`
	Immediate     types.Bool     `tfsdk:"immediate"`
	Plan          types.String   `tfsdk:"plan"`
	BillingCycle  types.String   `tfsdk:"billing_cycle"`
	PseudoService types.String   `tfsdk:"pseudo_service"`
	CloudProvider types.String   `tfsdk:"cloud_provider"`
	Region        types.String   `tfsdk:"region"`
	Project       types.String   `tfsdk:"project"`
	Timeouts      timeouts.Value `tfsdk:"timeouts"`
}

func NewVolumeBackupResource() resource.Resource {
	return &volumeBackupResource{}
}

func (r *volumeBackupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_volume_backup"
}

func (r *volumeBackupResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a scheduled backup for a ZCP block storage volume. The API has no update endpoint for backup schedules, so every change forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Backup slug.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"volume": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Slug of the block storage volume to back up. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"interval": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Backup interval (e.g. `dailyAt`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"at": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Hour at which the backup triggers (e.g. `1` for 1 AM). Defaults to `1`. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"immediate": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Run a backup immediately after creating the schedule. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"plan": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Backup plan slug (e.g. `backup-yow`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"billing_cycle": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Billing cycle (e.g. `hourly`, `monthly`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"pseudo_service": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Pseudo-service slug the backup bills under. Defaults to `" + defaultVolumeBackupPseudoService + "`. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
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
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Delete: true,
			}),
		},
	}
}

func (r *volumeBackupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = backup.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

func (r *volumeBackupResource) projectOrDefault(project types.String) string {
	if !project.IsNull() && !project.IsUnknown() {
		return project.ValueString()
	}
	return r.defaultProject
}

func (r *volumeBackupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model volumeBackupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_volume_backup cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 15*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	at := int64(1)
	if !model.At.IsNull() && !model.At.IsUnknown() {
		at = model.At.ValueInt64()
	}
	pseudoService := defaultVolumeBackupPseudoService
	if !model.PseudoService.IsNull() && !model.PseudoService.IsUnknown() {
		pseudoService = model.PseudoService.ValueString()
	}
	createReq := backup.CreateRequest{
		Interval:      model.Interval.ValueString(),
		At:            int(at),
		CloudProvider: model.CloudProvider.ValueString(),
		Region:        model.Region.ValueString(),
		BillingCycle:  model.BillingCycle.ValueString(),
		Plan:          model.Plan.ValueString(),
		PseudoService: pseudoService,
		Project:       r.projectOrDefault(model.Project),
	}
	if !model.Immediate.IsNull() && !model.Immediate.IsUnknown() && model.Immediate.ValueBool() {
		createReq.Immediate = 1
	}

	created, err := r.svc.Create(ctx, model.Volume.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create volume backup", err.Error())
		return
	}
	if created.Slug == "" {
		resp.Diagnostics.AddError(
			"Failed to resolve created volume backup",
			"the API accepted the backup but returned no slug.",
		)
		return
	}

	model.ID = types.StringValue(created.Slug)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *volumeBackupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model volumeBackupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_volume_backup cannot be read: bearer_token is missing.")
		return
	}

	backups, err := r.svc.List(ctx, model.Region.ValueString(), r.projectOrDefault(model.Project))
	if err != nil {
		resp.Diagnostics.AddError("Failed to read volume backup", err.Error())
		return
	}

	slug := model.ID.ValueString()
	for _, b := range backups {
		if b.Slug == slug {
			if b.Interval != "" {
				model.Interval = types.StringValue(b.Interval)
			}
			if !model.At.IsNull() {
				model.At = types.Int64Value(int64(b.At))
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *volumeBackupResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are ForceNew; Terraform never invokes this method.
}

func (r *volumeBackupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model volumeBackupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_volume_backup cannot be deleted: bearer_token is missing.")
		return
	}

	deleteTimeout, diags := model.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	slug := model.ID.ValueString()
	err := r.svc.Delete(deleteCtx, slug)
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete volume backup", err.Error())
		return
	}

	region := model.Region.ValueString()
	project := r.projectOrDefault(model.Project)
	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		backups, err := r.svc.List(ctx, region, project)
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for _, b := range backups {
			if b.Slug == slug {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError("Volume backup deletion did not complete", err.Error())
	}
}

// ImportState accepts a composite ID so the write-only scope fields are seeded
// for a zero-diff plan after import. Format:
//
//	<slug>/<region>/<cloud_provider>/<volume>/<interval>/<billing_cycle>[/<plan>/<project>]
func (r *volumeBackupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"id", "region", "cloud_provider", "volume", "interval", "billing_cycle", "plan", "project"}
	importPositional(ctx, req, resp, fields, 6,
		"<slug>/<region>/<cloud_provider>/<volume>/<interval>/<billing_cycle>[/<plan>/<project>]")
}
