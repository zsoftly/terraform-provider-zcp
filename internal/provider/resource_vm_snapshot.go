package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/vmsnapshot"
)

var _ resource.Resource = &vmSnapshotResource{}
var _ resource.ResourceWithConfigure = &vmSnapshotResource{}
var _ resource.ResourceWithImportState = &vmSnapshotResource{}

type vmSnapshotServiceIface interface {
	List(ctx context.Context, region, project string) ([]vmsnapshot.VMSnapshot, error)
	Create(ctx context.Context, vmSlug string, req vmsnapshot.CreateRequest) (*vmsnapshot.ActionResponse, error)
	Delete(ctx context.Context, snapshotSlug string) error
}

type vmSnapshotResource struct {
	svc            vmSnapshotServiceIface
	defaultProject string
}

type vmSnapshotResourceModel struct {
	ID             types.String   `tfsdk:"id"`
	VirtualMachine types.String   `tfsdk:"virtual_machine"`
	Name           types.String   `tfsdk:"name"`
	Plan           types.String   `tfsdk:"plan"`
	BillingCycle   types.String   `tfsdk:"billing_cycle"`
	Service        types.String   `tfsdk:"service"`
	IsMemory       types.Bool     `tfsdk:"is_memory"`
	CloudProvider  types.String   `tfsdk:"cloud_provider"`
	Region         types.String   `tfsdk:"region"`
	Project        types.String   `tfsdk:"project"`
	State          types.String   `tfsdk:"state"`
	Timeouts       timeouts.Value `tfsdk:"timeouts"`
}

func NewVMSnapshotResource() resource.Resource {
	return &vmSnapshotResource{}
}

func (r *vmSnapshotResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_snapshot"
}

func (r *vmSnapshotResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a point-in-time snapshot of a ZCP instance. Snapshots are immutable, so every change forces replacement. Reverting is an operational action outside Terraform (`zcp vm-snapshot revert`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "VM snapshot slug.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"virtual_machine": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Slug of the instance to snapshot. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Snapshot name. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"plan": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Snapshot plan slug (e.g. `vm-snapshot-yow`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"billing_cycle": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Billing cycle (e.g. `hourly`, `monthly`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"service": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Service slug the snapshot bills under (e.g. `virtual-machine`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"is_memory": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Include memory state in the snapshot. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
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
				MarkdownDescription: "Current state of the snapshot.",
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

func (r *vmSnapshotResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = vmsnapshot.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

func (r *vmSnapshotResource) projectOrDefault(project types.String) string {
	if !project.IsNull() && !project.IsUnknown() {
		return project.ValueString()
	}
	return r.defaultProject
}

func (r *vmSnapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model vmSnapshotResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vm_snapshot cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 15*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	region := model.Region.ValueString()
	project := r.projectOrDefault(model.Project)
	name := model.Name.ValueString()

	// The create endpoint returns only an action acknowledgement, so capture the
	// pre-create slugs and resolve the new snapshot from the list afterwards.
	before := map[string]bool{}
	if existing, err := r.svc.List(ctx, region, project); err == nil {
		for _, s := range existing {
			before[s.Slug] = true
		}
	}

	createReq := vmsnapshot.CreateRequest{
		Name:          name,
		BillingCycle:  model.BillingCycle.ValueString(),
		Plan:          model.Plan.ValueString(),
		IsVMSnapshot:  true,
		Project:       project,
		CloudProvider: model.CloudProvider.ValueString(),
		Region:        region,
		Service:       model.Service.ValueString(),
	}
	if !model.IsMemory.IsNull() && !model.IsMemory.IsUnknown() {
		createReq.IsMemory = model.IsMemory.ValueBool()
	}

	if _, err := r.svc.Create(ctx, model.VirtualMachine.ValueString(), createReq); err != nil {
		resp.Diagnostics.AddError("Failed to create VM snapshot", err.Error())
		return
	}

	var created *vmsnapshot.VMSnapshot
	if err := pollUntilReady(ctx, 5*time.Second, func(ctx context.Context) (bool, error) {
		snaps, err := r.svc.List(ctx, region, project)
		if err != nil {
			return false, err
		}
		for i := range snaps {
			if !before[snaps[i].Slug] && snaps[i].Name == name {
				created = &snaps[i]
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError(
			"VM snapshot did not appear after create",
			fmt.Sprintf("snapshot %q was accepted but never showed up in the snapshot list: %s", name, err),
		)
		return
	}

	model.ID = types.StringValue(created.Slug)
	if created.State != "" {
		model.State = types.StringValue(created.State)
	} else {
		model.State = types.StringNull()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *vmSnapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model vmSnapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vm_snapshot cannot be read: bearer_token is missing.")
		return
	}

	snaps, err := r.svc.List(ctx, model.Region.ValueString(), r.projectOrDefault(model.Project))
	if err != nil {
		resp.Diagnostics.AddError("Failed to read VM snapshot", err.Error())
		return
	}

	slug := model.ID.ValueString()
	for _, s := range snaps {
		if s.Slug == slug {
			model.Name = types.StringValue(s.Name)
			if s.State != "" {
				model.State = types.StringValue(s.State)
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *vmSnapshotResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are ForceNew; Terraform never invokes this method.
}

func (r *vmSnapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model vmSnapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vm_snapshot cannot be deleted: bearer_token is missing.")
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
		resp.Diagnostics.AddError("Failed to delete VM snapshot", err.Error())
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
		resp.Diagnostics.AddError("VM snapshot deletion did not complete", err.Error())
	}
}

// ImportState accepts a composite ID so the write-only scope fields are seeded
// for a zero-diff plan after import. Format:
//
//	<slug>/<region>/<cloud_provider>/<virtual_machine>/<billing_cycle>[/<plan>/<project>]
func (r *vmSnapshotResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"id", "region", "cloud_provider", "virtual_machine", "billing_cycle", "plan", "project"}
	importPositional(ctx, req, resp, fields, 5,
		"<slug>/<region>/<cloud_provider>/<virtual_machine>/<billing_cycle>[/<plan>/<project>]")
}
