package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/snapshot"
)

// volumeSnapshotService matches the CLI's fixed service value for block
// storage snapshots.
const volumeSnapshotService = "Block Storage Snapshot"

var _ resource.Resource = &volumeSnapshotResource{}
var _ resource.ResourceWithConfigure = &volumeSnapshotResource{}
var _ resource.ResourceWithImportState = &volumeSnapshotResource{}

type volumeSnapshotServiceIface interface {
	List(ctx context.Context, region, project string) ([]snapshot.Snapshot, error)
	Create(ctx context.Context, blockstorageSlug string, req snapshot.CreateRequest) (*snapshot.Snapshot, error)
	Delete(ctx context.Context, slug string) error
}

type volumeSnapshotResource struct {
	svc            volumeSnapshotServiceIface
	defaultProject string
}

type volumeSnapshotResourceModel struct {
	ID            types.String   `tfsdk:"id"`
	Volume        types.String   `tfsdk:"volume"`
	Name          types.String   `tfsdk:"name"`
	Plan          types.String   `tfsdk:"plan"`
	BillingCycle  types.String   `tfsdk:"billing_cycle"`
	CloudProvider types.String   `tfsdk:"cloud_provider"`
	Region        types.String   `tfsdk:"region"`
	Project       types.String   `tfsdk:"project"`
	Timeouts      timeouts.Value `tfsdk:"timeouts"`
}

func NewVolumeSnapshotResource() resource.Resource {
	return &volumeSnapshotResource{}
}

func (r *volumeSnapshotResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_volume_snapshot"
}

func (r *volumeSnapshotResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a snapshot of a ZCP block storage volume. Snapshots are immutable, so every change forces replacement. Reverting is an operational action outside Terraform (`zcp snapshot revert`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Snapshot slug.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"volume": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Slug of the block storage volume to snapshot. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Snapshot name. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"plan": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Snapshot plan slug. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"billing_cycle": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Billing cycle (e.g. `hourly`, `monthly`). Changing this forces replacement.",
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

func (r *volumeSnapshotResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = snapshot.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

func (r *volumeSnapshotResource) projectOrDefault(project types.String) string {
	if !project.IsNull() && !project.IsUnknown() {
		return project.ValueString()
	}
	return r.defaultProject
}

func (r *volumeSnapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model volumeSnapshotResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_volume_snapshot cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 15*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	name := model.Name.ValueString()
	region := model.Region.ValueString()
	project := r.projectOrDefault(model.Project)

	created, err := r.svc.Create(ctx, model.Volume.ValueString(), snapshot.CreateRequest{
		Name:          name,
		Plan:          model.Plan.ValueString(),
		Service:       volumeSnapshotService,
		Project:       project,
		CloudProvider: model.CloudProvider.ValueString(),
		Region:        region,
		BillingCycle:  model.BillingCycle.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create volume snapshot", err.Error())
		return
	}

	slug := created.Slug
	if slug == "" {
		// Some create responses return a partial object; resolve the slug from
		// the list by name so state never stores an empty ID.
		if err := pollUntilReady(ctx, 5*time.Second, func(ctx context.Context) (bool, error) {
			snaps, lerr := r.svc.List(ctx, region, project)
			if lerr != nil {
				return false, lerr
			}
			for _, s := range snaps {
				if s.Name == name {
					slug = s.Slug
					return true, nil
				}
			}
			return false, nil
		}); err != nil {
			resp.Diagnostics.AddError(
				"Volume snapshot did not appear after create",
				fmt.Sprintf("snapshot %q was accepted but never showed up in the snapshot list: %s", name, err),
			)
			return
		}
	}

	model.ID = types.StringValue(slug)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *volumeSnapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model volumeSnapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_volume_snapshot cannot be read: bearer_token is missing.")
		return
	}

	snaps, err := r.svc.List(ctx, model.Region.ValueString(), r.projectOrDefault(model.Project))
	if err != nil {
		resp.Diagnostics.AddError("Failed to read volume snapshot", err.Error())
		return
	}

	slug := model.ID.ValueString()
	for _, s := range snaps {
		if s.Slug == slug {
			model.Name = types.StringValue(s.Name)
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *volumeSnapshotResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are ForceNew; Terraform never invokes this method.
}

func (r *volumeSnapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model volumeSnapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_volume_snapshot cannot be deleted: bearer_token is missing.")
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
		resp.Diagnostics.AddError("Failed to delete volume snapshot", err.Error())
		return
	}

	region := model.Region.ValueString()
	project := r.projectOrDefault(model.Project)
	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		snaps, err := r.svc.List(ctx, region, project)
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for _, s := range snaps {
			if s.Slug == slug {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError("Volume snapshot deletion did not complete", err.Error())
	}
}

// ImportState accepts a composite ID so the write-only scope fields are seeded
// for a zero-diff plan after import. Format:
//
//	<slug>/<region>/<cloud_provider>/<volume>/<billing_cycle>[/<plan>/<project>]
func (r *volumeSnapshotResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"id", "region", "cloud_provider", "volume", "billing_cycle", "plan", "project"}
	importPositional(ctx, req, resp, fields, 5,
		"<slug>/<region>/<cloud_provider>/<volume>/<billing_cycle>[/<plan>/<project>]")
}
