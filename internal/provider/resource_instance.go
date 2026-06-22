package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/instance"
)

var _ resource.Resource = &instanceResource{}
var _ resource.ResourceWithConfigure = &instanceResource{}
var _ resource.ResourceWithImportState = &instanceResource{}

type instanceServiceIface interface {
	Create(ctx context.Context, req instance.CreateRequest) (*instance.VirtualMachine, error)
	Get(ctx context.Context, slug string) (*instance.VirtualMachine, error)
	WaitForState(ctx context.Context, slug string, targetStates []string, pollInterval time.Duration) (*instance.VirtualMachine, error)
	ChangeHostname(ctx context.Context, slug string, req instance.ChangeLabelRequest) (*instance.ActionResponse, error)
	ChangePlan(ctx context.Context, slug string, req instance.ChangePlanRequest) (*instance.ActionResponse, error)
	ChangeStartupScript(ctx context.Context, slug string, req instance.ChangeStartupScriptRequest) (*instance.ActionResponse, error)
	CreateTag(ctx context.Context, slug string, req instance.TagRequest) (*instance.ActionResponse, error)
	DeleteTag(ctx context.Context, slug string, key string) error
	Start(ctx context.Context, slug string) (*instance.ActionResponse, error)
	Stop(ctx context.Context, slug string) (*instance.ActionResponse, error)
	Delete(ctx context.Context, slug string, expunge bool) error
}

type instanceResource struct {
	svc            instanceServiceIface
	defaultProject string
}

type instanceResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	CloudProvider   types.String `tfsdk:"cloud_provider"`
	Region          types.String `tfsdk:"region"`
	Template        types.String `tfsdk:"template"`
	Plan            types.String `tfsdk:"plan"`
	BillingCycle    types.String `tfsdk:"billing_cycle"`
	Project         types.String `tfsdk:"project"`
	SSHKey          types.String `tfsdk:"ssh_key"`
	NetworkPlan     types.String `tfsdk:"network_plan"`
	StorageCategory types.String `tfsdk:"storage_category"`
	UserData        types.String `tfsdk:"user_data"`
	Tags            types.Map    `tfsdk:"tags"`
	PowerState      types.String `tfsdk:"power_state"`
	// Computed
	Slug      types.String   `tfsdk:"slug"`
	State     types.String   `tfsdk:"state"`
	PrivateIP types.String   `tfsdk:"private_ip"`
	PublicIP  types.String   `tfsdk:"public_ip"`
	Timeouts  timeouts.Value `tfsdk:"timeouts"`
}

func NewInstanceResource() resource.Resource {
	return &instanceResource{}
}

func (r *instanceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_instance"
}

func (r *instanceResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	useStateForUnknown := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP virtual machine instance. Create blocks until the instance reaches the `Running` state. `name`, `plan`, `billing_cycle`, `user_data`, `tags`, and `power_state` are updated in place (mirroring the CLI `change-*`, tag, and start/stop operations); `cloud_provider`, `region`, and `template` force replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Instance slug (unique identifier).",
				PlanModifiers:       useStateForUnknown,
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name (and hostname) of the instance. Updated in place via the change-hostname operation.",
			},
			"cloud_provider": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cloud provider slug. Use `data.zcp_region.<name>.cloud_provider` instead of hardcoding. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Region slug where the instance is created (e.g. `yow-1`). Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"template": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Template (OS image) slug. See `data.zcp_template`. Changing this forces replacement (an OS change reprovisions the disk).",
				PlanModifiers:       requiresReplace,
			},
			"plan": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Compute plan slug. Run `zcp plan vm` to list available plans. Updated in place (resize) via the change-plan operation.",
			},
			"billing_cycle": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Billing cycle (`hourly` or `monthly`). Updated in place together with `plan`.",
			},
			"project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project slug. Inherits from the provider `default_project` if omitted. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"ssh_key": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Name of an existing SSH key to attach for login (see `zcp_ssh_key`). Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"network_plan": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Network plan slug (e.g. `pnet-yow`). Required by the public API. Run `zcp plan network` to list values. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"storage_category": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Storage category slug (e.g. `nvme`, `pro-nvme`). Required by the public API. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"user_data": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Startup script content (cloud-init / bash). Updated in place via the change-startup-script operation (takes effect on next boot).",
			},
			"tags": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Key/value tags applied via the tag-create / tag-delete operations. Write-only: the ZCP API does not return tags on read, so they are tracked in state but not refreshed (no drift detection) and are not populated on import.",
			},
			"power_state": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Desired power state: `running` or `stopped`. When set, the provider enforces it via start/stop; when omitted it simply reflects the instance's current state. Defaults to `running` on create.",
				PlanModifiers:       useStateForUnknown,
				Validators:          []validator.String{powerStateValidator{}},
			},
			"slug": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Instance slug (same value as `id`).",
				PlanModifiers:       useStateForUnknown,
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current instance state (e.g. `Running`).",
			},
			"private_ip": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Private IP address of the instance's default network.",
			},
			"public_ip": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Public IP address of the instance, if assigned.",
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

func (r *instanceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = instance.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

// normalizePowerState maps a CloudStack VM state to a power_state value. Only
// running/stopped are valid user inputs; any other (transient) state is
// lowercased so the computed attribute always holds a known value.
func normalizePowerState(state string) string {
	switch strings.ToLower(state) {
	case "running":
		return "running"
	case "stopped":
		return "stopped"
	default:
		return strings.ToLower(state)
	}
}

// applyVMState populates the computed attributes (id/slug/state/IPs/power_state)
// from a VM. It does NOT touch tags (the API never returns them).
func (r *instanceResource) applyVMState(model *instanceResourceModel, vm *instance.VirtualMachine) {
	model.ID = types.StringValue(vm.Slug)
	model.Slug = types.StringValue(vm.Slug)
	model.State = types.StringValue(vm.State)
	model.PrivateIP = types.StringValue(vm.NetworkPrivateIP())
	model.PublicIP = types.StringValue(instance.StringVal(vm.PublicIP))
	model.PowerState = types.StringValue(normalizePowerState(vm.State))
}

func (r *instanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model instanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_instance cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 20*time.Minute)
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

	createReq := instance.CreateRequest{
		Name:            model.Name.ValueString(),
		CloudProvider:   model.CloudProvider.ValueString(),
		Project:         project,
		Region:          model.Region.ValueString(),
		BootSource:      "image",
		Server:          "cloud-compute",
		Template:        model.Template.ValueString(),
		IsPublic:        true,
		NetworkType:     "Isolated",
		Networks:        []string{},
		BillingCycle:    model.BillingCycle.ValueString(),
		Plan:            model.Plan.ValueString(),
		OSFamily:        "Linux",
		TemplateType:    "Operating System",
		Hostname:        model.Name.ValueString(),
		Addons:          []string{},
		StorageCategory: model.StorageCategory.ValueString(),
		NetworkPlan:     model.NetworkPlan.ValueString(),
	}
	if sshKey := model.SSHKey.ValueString(); sshKey != "" {
		createReq.SSHKey = &sshKey
		createReq.AuthMethod = "ssh-key"
		empty := ""
		createReq.Password = &empty
	}
	if userData := model.UserData.ValueString(); userData != "" {
		createReq.UserData = &userData
	}

	vm, err := r.svc.Create(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create instance", err.Error())
		return
	}
	slug := vm.Slug

	// Block until Running so both IPs are populated (mirrors the CLI --wait).
	ready, err := r.svc.WaitForState(ctx, slug, []string{"Running"}, 0)
	if err != nil {
		resp.Diagnostics.AddError(
			"Instance did not reach Running",
			fmt.Sprintf("instance %s was created but did not become Running: %s", slug, err.Error()),
		)
		return
	}

	// Apply tags requested at create time.
	tags, tdiags := mapToStringMap(ctx, model.Tags)
	resp.Diagnostics.Append(tdiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	for k, v := range tags {
		if _, err := r.svc.CreateTag(ctx, slug, instance.TagRequest{Key: k, Value: v}); err != nil {
			resp.Diagnostics.AddError("Failed to apply instance tag", fmt.Sprintf("tag %q: %s", k, err.Error()))
			return
		}
	}

	// If the user explicitly requested a stopped instance, stop it now.
	if model.PowerState.ValueString() == "stopped" {
		if _, err := r.svc.Stop(ctx, slug); err != nil {
			resp.Diagnostics.AddError("Failed to stop instance", err.Error())
			return
		}
		ready, err = r.svc.WaitForState(ctx, slug, []string{"Stopped"}, 0)
		if err != nil {
			resp.Diagnostics.AddError("Instance did not reach Stopped", err.Error())
			return
		}
	}

	r.applyVMState(&model, ready)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *instanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model instanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_instance cannot be read: bearer_token is missing.")
		return
	}

	vm, err := r.svc.Get(ctx, model.ID.ValueString())
	if err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read instance", err.Error())
		return
	}

	model.Name = types.StringValue(vm.Name)
	r.applyVMState(&model, vm)
	// tags are not refreshed (the API does not return them); preserved from state.
	// cloud_provider, region, template, plan, billing_cycle, project, ssh_key,
	// network_plan, storage_category, user_data are create/update inputs preserved
	// from state (not reliably echoed by the API in a comparable form).
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *instanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan instanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state instanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_instance cannot be updated: bearer_token is missing.")
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

	// The instance should end in this power state. power_state is Optional+Computed
	// with UseStateForUnknown, so the plan value is always known here — either what
	// the user set or the carried-over current state. Resizing is transparent: the
	// VM is stopped to change its compute offering and then restored to this state
	// (mirroring how aws_instance handles an instance_type change), so users never
	// manage power just to resize.
	desiredPower := plan.PowerState.ValueString()
	if desiredPower == "" {
		desiredPower = "running"
	}

	// 1. Rename (change-hostname sets both name and hostname).
	if !plan.Name.Equal(state.Name) {
		if _, err := r.svc.ChangeHostname(ctx, slug, instance.ChangeLabelRequest{
			Name:     plan.Name.ValueString(),
			Hostname: plan.Name.ValueString(),
		}); err != nil {
			resp.Diagnostics.AddError("Failed to rename instance", err.Error())
			return
		}
	}

	// 2. Startup script.
	if !plan.UserData.Equal(state.UserData) {
		if _, err := r.svc.ChangeStartupScript(ctx, slug, instance.ChangeStartupScriptRequest{
			UserData: plan.UserData.ValueString(),
		}); err != nil {
			resp.Diagnostics.AddError("Failed to update startup script", err.Error())
			return
		}
	}

	// 3. Plan / billing cycle (resize) — stop, change the offering, then let the
	//    power-state reconciliation below restart it if it was running. Changing a
	//    CloudStack compute offering requires the VM to be stopped.
	if !plan.Plan.Equal(state.Plan) || !plan.BillingCycle.Equal(state.BillingCycle) {
		if err := r.ensureStopped(ctx, slug); err != nil {
			resp.Diagnostics.AddError("Failed to stop instance for resize", err.Error())
			return
		}
		if _, err := r.svc.ChangePlan(ctx, slug, instance.ChangePlanRequest{
			Plan:         plan.Plan.ValueString(),
			Slug:         slug,
			VM:           slug,
			BillingCycle: plan.BillingCycle.ValueString(),
		}); err != nil {
			resp.Diagnostics.AddError("Failed to change instance plan", err.Error())
			return
		}
		if _, err := r.svc.WaitForState(ctx, slug, []string{"Stopped"}, 0); err != nil {
			resp.Diagnostics.AddError("Instance did not settle after plan change", err.Error())
			return
		}
	}

	// 4. Tags (create/update changed keys, delete removed keys).
	if !plan.Tags.Equal(state.Tags) {
		if diags := r.reconcileTags(ctx, slug, state.Tags, plan.Tags); diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	}

	// 5. Reconcile to the desired power state (also restores a VM stopped for resize).
	if err := r.ensurePowerState(ctx, slug, desiredPower); err != nil {
		resp.Diagnostics.AddError("Failed to reach desired power state", err.Error())
		return
	}

	// Re-read for consistent computed values (state, IPs, power_state).
	vm, err := r.svc.Get(ctx, slug)
	if err != nil {
		resp.Diagnostics.AddError("Failed to refresh instance after update", err.Error())
		return
	}
	r.applyVMState(&plan, vm)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// ensureStopped stops the instance if it is not already stopped and waits for it
// to reach the Stopped state.
func (r *instanceResource) ensureStopped(ctx context.Context, slug string) error {
	vm, err := r.svc.Get(ctx, slug)
	if err != nil {
		return err
	}
	if normalizePowerState(vm.State) == "stopped" {
		return nil
	}
	if _, err := r.svc.Stop(ctx, slug); err != nil {
		return err
	}
	_, err = r.svc.WaitForState(ctx, slug, []string{"Stopped"}, 0)
	return err
}

// ensurePowerState drives the instance to the requested power state
// ("running" or "stopped"), waiting for it to settle. It is a no-op when the
// instance is already there.
func (r *instanceResource) ensurePowerState(ctx context.Context, slug, desired string) error {
	vm, err := r.svc.Get(ctx, slug)
	if err != nil {
		return err
	}
	current := normalizePowerState(vm.State)
	switch desired {
	case "stopped":
		if current == "stopped" {
			return nil
		}
		if _, err := r.svc.Stop(ctx, slug); err != nil {
			return err
		}
		_, err = r.svc.WaitForState(ctx, slug, []string{"Stopped"}, 0)
		return err
	default: // running
		if current == "running" {
			return nil
		}
		if _, err := r.svc.Start(ctx, slug); err != nil {
			return err
		}
		_, err = r.svc.WaitForState(ctx, slug, []string{"Running"}, 0)
		return err
	}
}

// mapToStringMap converts a (possibly null/unknown) types.Map of strings into a
// plain map.
func mapToStringMap(ctx context.Context, m types.Map) (map[string]string, diag.Diagnostics) {
	out := map[string]string{}
	if m.IsNull() || m.IsUnknown() {
		return out, nil
	}
	diags := m.ElementsAs(ctx, &out, false)
	return out, diags
}

// reconcileTags upserts changed/new tags and deletes removed ones.
func (r *instanceResource) reconcileTags(ctx context.Context, slug string, stateTags, planTags types.Map) diag.Diagnostics {
	var diags diag.Diagnostics
	current, d := mapToStringMap(ctx, stateTags)
	diags.Append(d...)
	desired, d := mapToStringMap(ctx, planTags)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	for k, v := range desired {
		if cur, ok := current[k]; !ok || cur != v {
			if _, err := r.svc.CreateTag(ctx, slug, instance.TagRequest{Key: k, Value: v}); err != nil {
				diags.AddError("Failed to set instance tag", fmt.Sprintf("tag %q: %s", k, err.Error()))
				return diags
			}
		}
	}
	for k := range current {
		if _, ok := desired[k]; !ok {
			if err := r.svc.DeleteTag(ctx, slug, k); err != nil && !apierrors.IsNotFound(err) {
				diags.AddError("Failed to delete instance tag", fmt.Sprintf("tag %q: %s", k, err.Error()))
				return diags
			}
		}
	}
	return diags
}

func (r *instanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model instanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_instance cannot be deleted: bearer_token is missing.")
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
	// expunge=true forces an immediate purge so the slug does not linger in a
	// soft-deleted state (which would otherwise make pollUntilGone time out).
	err := r.svc.Delete(deleteCtx, slug, true)
	if err != nil && !apierrors.IsNotFound(err) && !apierrors.IsResourceNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete instance", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		_, err := r.svc.Get(ctx, slug)
		if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
			return false, nil
		}
		return err == nil, err
	}); err != nil {
		resp.Diagnostics.AddError("Instance deletion did not complete", err.Error())
	}
}

// ImportState seeds the create-only attributes the API does not return in a
// comparable form so a post-import plan is zero-diff. Format (slash-separated,
// trailing/empty segments allowed):
//
//	<slug>/<cloud_provider>/<region>/<template>/<plan>/<billing_cycle>/<project>/<ssh_key>/<network_plan>/<storage_category>
//
// <slug>/<cloud_provider>/<region>/<template> are required. name, state, IPs and
// power_state come from the subsequent Read; tags cannot be imported (the API
// does not return them).
func (r *instanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"id", "cloud_provider", "region", "template", "plan", "billing_cycle", "project", "ssh_key", "network_plan", "storage_category"}
	importPositional(ctx, req, resp, fields, 4,
		"<slug>/<cloud_provider>/<region>/<template>[/<plan>/<billing_cycle>/<project>/<ssh_key>/<network_plan>/<storage_category>]")
}
