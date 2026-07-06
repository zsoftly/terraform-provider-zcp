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
	"github.com/zsoftly/zcp-cli/pkg/api/vmbackup"
)

// defaultVMBackupPseudoService matches the CLI's documented value for
// --pseudo-service on `zcp vm-backup create`.
const defaultVMBackupPseudoService = "vm-backup"

var _ resource.Resource = &vmBackupResource{}
var _ resource.ResourceWithConfigure = &vmBackupResource{}
var _ resource.ResourceWithImportState = &vmBackupResource{}

type vmBackupServiceIface interface {
	List(ctx context.Context, region, project string) ([]vmbackup.VMBackup, error)
	Create(ctx context.Context, vmSlug string, req vmbackup.CreateRequest) (*vmbackup.ActionResponse, error)
	Delete(ctx context.Context, slug string) error
}

type vmBackupResource struct {
	svc            vmBackupServiceIface
	defaultProject string
}

type vmBackupResourceModel struct {
	ID             types.String   `tfsdk:"id"`
	VirtualMachine types.String   `tfsdk:"virtual_machine"`
	Interval       types.String   `tfsdk:"interval"`
	At             types.Int64    `tfsdk:"at"`
	Immediate      types.Bool     `tfsdk:"immediate"`
	Plan           types.String   `tfsdk:"plan"`
	BillingCycle   types.String   `tfsdk:"billing_cycle"`
	PseudoService  types.String   `tfsdk:"pseudo_service"`
	CloudProvider  types.String   `tfsdk:"cloud_provider"`
	Region         types.String   `tfsdk:"region"`
	Project        types.String   `tfsdk:"project"`
	State          types.String   `tfsdk:"state"`
	Timeouts       timeouts.Value `tfsdk:"timeouts"`
}

func NewVMBackupResource() resource.Resource {
	return &vmBackupResource{}
}

func (r *vmBackupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_backup"
}

func (r *vmBackupResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a scheduled backup for a ZCP instance. The API has no update endpoint for backup schedules, so every change forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "VM backup slug.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"virtual_machine": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Slug of the instance to back up. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"interval": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Backup interval (e.g. `daily`, `weekly`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"at": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Hour of day for the scheduled backup (0-23). Defaults to `0`. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"immediate": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Run a backup immediately after creating the schedule. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"plan": schema.StringAttribute{
				Required:            true,
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
				MarkdownDescription: "Pseudo-service slug the backup bills under. Defaults to `" + defaultVMBackupPseudoService + "`. Changing this forces replacement.",
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
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current state of the backup.",
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

func (r *vmBackupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = vmbackup.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

func (r *vmBackupResource) projectOrDefault(project types.String) string {
	if !project.IsNull() && !project.IsUnknown() {
		return project.ValueString()
	}
	return r.defaultProject
}

func (r *vmBackupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model vmBackupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vm_backup cannot be created: bearer_token is missing.")
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

	// The create endpoint returns only an action acknowledgement, so capture the
	// pre-create slugs and resolve the new backup from the list afterwards.
	before := map[string]bool{}
	if existing, err := r.svc.List(ctx, region, project); err == nil {
		for _, b := range existing {
			before[b.Slug] = true
		}
	}

	pseudoService := defaultVMBackupPseudoService
	if !model.PseudoService.IsNull() && !model.PseudoService.IsUnknown() {
		pseudoService = model.PseudoService.ValueString()
	}
	createReq := vmbackup.CreateRequest{
		Interval:      model.Interval.ValueString(),
		At:            int(model.At.ValueInt64()),
		CloudProvider: model.CloudProvider.ValueString(),
		Region:        region,
		BillingCycle:  model.BillingCycle.ValueString(),
		Plan:          model.Plan.ValueString(),
		PseudoService: pseudoService,
		Project:       project,
	}
	if !model.Immediate.IsNull() && !model.Immediate.IsUnknown() && model.Immediate.ValueBool() {
		createReq.Immediate = 1
	}

	if _, err := r.svc.Create(ctx, model.VirtualMachine.ValueString(), createReq); err != nil {
		resp.Diagnostics.AddError("Failed to create VM backup", err.Error())
		return
	}

	var created *vmbackup.VMBackup
	if err := pollUntilReady(ctx, 5*time.Second, func(ctx context.Context) (bool, error) {
		backups, err := r.svc.List(ctx, region, project)
		if err != nil {
			return false, err
		}
		for i := range backups {
			if !before[backups[i].Slug] {
				created = &backups[i]
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError(
			"VM backup did not appear after create",
			fmt.Sprintf("the backup for %q was accepted but never showed up in the backup list: %s", model.VirtualMachine.ValueString(), err),
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

func (r *vmBackupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model vmBackupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vm_backup cannot be read: bearer_token is missing.")
		return
	}

	backups, err := r.svc.List(ctx, model.Region.ValueString(), r.projectOrDefault(model.Project))
	if err != nil {
		resp.Diagnostics.AddError("Failed to read VM backup", err.Error())
		return
	}

	slug := model.ID.ValueString()
	for _, b := range backups {
		if b.Slug == slug {
			if b.State != "" {
				model.State = types.StringValue(b.State)
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *vmBackupResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are ForceNew; Terraform never invokes this method.
}

func (r *vmBackupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model vmBackupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vm_backup cannot be deleted: bearer_token is missing.")
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
		resp.Diagnostics.AddError("Failed to delete VM backup", err.Error())
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
		resp.Diagnostics.AddError("VM backup deletion did not complete", err.Error())
	}
}

// ImportState accepts a composite ID so the write-only scope fields are seeded
// for a zero-diff plan after import. Format:
//
//	<slug>/<region>/<cloud_provider>/<virtual_machine>/<interval>/<plan>/<billing_cycle>[/<project>]
func (r *vmBackupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"id", "region", "cloud_provider", "virtual_machine", "interval", "plan", "billing_cycle", "project"}
	importPositional(ctx, req, resp, fields, 7,
		"<slug>/<region>/<cloud_provider>/<virtual_machine>/<interval>/<plan>/<billing_cycle>[/<project>]")
}
