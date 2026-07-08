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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/instance"
	"github.com/zsoftly/zcp-cli/pkg/api/ipaddress"
	"github.com/zsoftly/zcp-cli/pkg/httpclient"
)

var _ resource.Resource = &instanceResource{}
var _ resource.ResourceWithConfigure = &instanceResource{}
var _ resource.ResourceWithImportState = &instanceResource{}
var _ resource.ResourceWithValidateConfig = &instanceResource{}

type instanceServiceIface interface {
	Create(ctx context.Context, req instance.CreateRequest) (*instance.VirtualMachine, error)
	Get(ctx context.Context, slug string) (*instance.VirtualMachine, error)
	Meta(ctx context.Context, slug string) (*instance.VMMeta, error)
	ActivityLogs(ctx context.Context, slug string) ([]instance.ActivityLog, error)
	ChangeLabel(ctx context.Context, slug, name string) error
	ChangePlan(ctx context.Context, slug string, req instance.ChangePlanRequest) (*instance.ActionResponse, error)
	ChangeStartupScript(ctx context.Context, slug string, req instance.ChangeStartupScriptRequest) (*instance.ActionResponse, error)
	CreateTag(ctx context.Context, slug string, req instance.TagRequest) (*instance.ActionResponse, error)
	DeleteTag(ctx context.Context, slug string, key string) error
	Start(ctx context.Context, slug string) (*instance.ActionResponse, error)
	Stop(ctx context.Context, slug string) (*instance.ActionResponse, error)
	Delete(ctx context.Context, slug string, expunge, deletePublicIP bool) error
}

// instanceService adapts the zcp-cli instance.Service, overriding the rename
// operation. The released CLI's ChangeHostname posts {name, hostname} to
// /change-label, but the API requires the field `vm_label` (its validation
// rejects the CLI payload with "The vm label field is required" — a known,
// unfixed CLI bug). We post the correct body directly via the shared HTTP client
// so display-name changes apply in place.
type instanceService struct {
	*instance.Service
	client *httpclient.Client
}

func (s *instanceService) ChangeLabel(ctx context.Context, slug, name string) error {
	body := map[string]string{"vm_label": name}
	var resp instance.ActionResponse
	if err := s.client.Post(ctx, "/virtual-machines/"+slug+"/change-label", body, &resp); err != nil {
		return err
	}
	return nil
}

// publicIPLister resolves a VM's public address from the account IP list. With
// source-NAT networks the address belongs to the network, so the VM object's
// public_ip stays empty (platform behavior; the CLI has the same gotcha), but
// the IP list associates the address with the VM.
type publicIPLister interface {
	List(ctx context.Context, vpcSlug, region, project string) ([]ipaddress.IPAddress, error)
}

type instanceResource struct {
	svc            instanceServiceIface
	ipSvc          publicIPLister
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
	Network         types.String `tfsdk:"network"`
	NetworkPlan     types.String `tfsdk:"network_plan"`
	AssignPublicIP  types.Bool   `tfsdk:"assign_public_ip"`
	StorageCategory types.String `tfsdk:"storage_category"`
	UserData        types.String `tfsdk:"user_data"`
	Tags            types.Map    `tfsdk:"tags"`
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
		MarkdownDescription: "Manages a ZCP virtual machine instance. Create blocks until the instance reaches the `Running` state. `name`, `plan`, `billing_cycle`, `user_data`, and `tags` are updated in place; `cloud_provider`, `region`, and `template` force replacement. The instance's runtime power state is not managed by Terraform — it is reported read-only in `state`, and the provider only stops/starts the VM internally when a resize requires it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Instance slug (unique identifier).",
				PlanModifiers:       useStateForUnknown,
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name of the instance. Updated in place. (The instance's hostname is set from this value at creation and, like all clouds, is not changed afterward.)",
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
			"network": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Slug of an existing `zcp_network` to attach the instance to. Mutually exclusive with `network_plan`. Prefer this: the instance attaches to a network you manage, so `terraform destroy` leaves nothing behind. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"network_plan": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Network plan slug (e.g. `pnet-yow`) used to auto-create an isolated network for the instance. Mutually exclusive with `network`. Note: the auto-created network is not managed by Terraform and is not removed on destroy — prefer `network` with an explicit `zcp_network`. Run `zcp plan network` to list values. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"assign_public_ip": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether to assign a public IP to the instance. Defaults to `true`. Set to `false` for a private-only instance. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"storage_category": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Storage category slug. Region-specific: `nvme`/`hdd-storage` in yow-1, `pro-nvme`/`premium-ssd` in yul-1. Required by the public API. Changing this forces replacement.",
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
			"slug": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Instance slug (same value as `id`).",
				PlanModifiers:       useStateForUnknown,
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current runtime state of the instance (e.g. `Running`, `Stopped`), reported for information only — Terraform does not reconcile or manage power state.",
			},
			"private_ip": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Private IP address of the instance's default network.",
			},
			"public_ip": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Public IP address of the instance, if assigned. When the address belongs to the network (source-NAT), the provider resolves it from the account IP list, since the platform leaves it off the VM object.",
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
	r.svc = &instanceService{Service: instance.NewService(pd.Client), client: pd.Client}
	r.ipSvc = ipaddress.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

// isRunning reports whether a CloudStack VM state string means the VM is running.
func isRunning(state string) bool {
	return strings.EqualFold(state, "running")
}

// applyVMState populates the computed attributes (id/slug/state/IPs) from a VM.
// It does NOT touch tags (the API never returns them) and does not manage power.
// Post-Running private-IP wait tunables. The platform records the private IP
// shortly after the VM reaches Running; vars so tests can shorten the wait.
var (
	privateIPPollInterval = 10 * time.Second
	privateIPPollWindow   = 2 * time.Minute
)

// resolvePublicIP returns the VM's public address from the IP list, preferring
// a static assignment over the network's source-NAT address. Best effort: ""
// when the lister is unavailable, errors, or finds nothing.
func (r *instanceResource) resolvePublicIP(ctx context.Context, vmID, region, project string) string {
	if r.ipSvc == nil || vmID == "" {
		return ""
	}
	ips, err := r.ipSvc.List(ctx, "", region, project)
	if err != nil {
		return ""
	}
	sourceNAT := ""
	for i := range ips {
		if ips[i].VirtualMachineID != vmID || ips[i].IPAddress == "" {
			continue
		}
		if !strings.EqualFold(ips[i].Strategy, "source-nat") {
			return ips[i].IPAddress
		}
		if sourceNAT == "" {
			sourceNAT = ips[i].IPAddress
		}
	}
	return sourceNAT
}

// fillPublicIP resolves public_ip from the IP list when the VM object carries
// none. Skipped when the config opted out of a public IP.
func (r *instanceResource) fillPublicIP(ctx context.Context, model *instanceResourceModel, vm *instance.VirtualMachine) {
	if model.PublicIP.ValueString() != "" {
		return
	}
	if !model.AssignPublicIP.IsNull() && !model.AssignPublicIP.IsUnknown() && !model.AssignPublicIP.ValueBool() {
		return
	}
	project := r.defaultProject
	if !model.Project.IsNull() && !model.Project.IsUnknown() {
		project = model.Project.ValueString()
	}
	if ip := r.resolvePublicIP(ctx, vm.ID, model.Region.ValueString(), project); ip != "" {
		model.PublicIP = types.StringValue(ip)
	}
}

func (r *instanceResource) applyVMState(model *instanceResourceModel, vm *instance.VirtualMachine) {
	model.ID = types.StringValue(vm.Slug)
	model.Slug = types.StringValue(vm.Slug)
	model.State = types.StringValue(vm.State)
	model.PrivateIP = types.StringValue(vm.NetworkPrivateIP())
	model.PublicIP = types.StringValue(instance.StringVal(vm.PublicIP))
}

// ValidateConfig enforces that `network` and `network_plan` are not both set:
// `network` attaches to an existing network, `network_plan` auto-creates one, so
// they are mutually exclusive.
func (r *instanceResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var model instanceResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	networkSet := !model.Network.IsNull() && !model.Network.IsUnknown()
	planSet := !model.NetworkPlan.IsNull() && !model.NetworkPlan.IsUnknown()
	if networkSet && planSet {
		resp.Diagnostics.AddError(
			"Conflicting network configuration",
			"`network` (attach to an existing network) and `network_plan` (auto-create a network) are mutually exclusive — set only one.",
		)
	}
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

	// assign_public_ip defaults to true (current behaviour) when unset.
	isPublic := model.AssignPublicIP.IsNull() || model.AssignPublicIP.IsUnknown() || model.AssignPublicIP.ValueBool()

	createReq := instance.CreateRequest{
		Name:            model.Name.ValueString(),
		CloudProvider:   model.CloudProvider.ValueString(),
		Project:         project,
		Region:          model.Region.ValueString(),
		BootSource:      "image",
		Server:          "cloud-compute",
		Template:        model.Template.ValueString(),
		IsPublic:        isPublic,
		NetworkType:     "Isolated",
		Networks:        []string{},
		BillingCycle:    model.BillingCycle.ValueString(),
		Plan:            model.Plan.ValueString(),
		OSFamily:        "Linux",
		TemplateType:    "Operating System",
		Hostname:        model.Name.ValueString(),
		Addons:          []string{},
		StorageCategory: model.StorageCategory.ValueString(),
	}
	// Attach to an existing network when `network` is set (no untracked
	// auto-created network); otherwise auto-create one from `network_plan`.
	if net := model.Network.ValueString(); net != "" {
		createReq.Networks = []string{net}
	} else {
		createReq.NetworkPlan = model.NetworkPlan.ValueString()
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

	// Block until Running so both IPs are populated (mirrors the CLI --wait). This
	// fails fast on a terminal provisioning state instead of blocking for the full
	// create timeout. If the VM never becomes usable it is already provisioned but
	// will not be saved to state, so clean it up to avoid an unmanaged orphan.
	ready, err := r.waitForRunning(ctx, slug)
	if err != nil {
		r.cleanupAfterFailedCreate(ctx, slug, isPublic, &resp.Diagnostics)
		resp.Diagnostics.AddError(
			"Instance did not reach Running",
			fmt.Sprintf("instance %s was created but did not become Running: %s", slug, err.Error()),
		)
		return
	}

	// Apply tags requested at create time. A tag failure does NOT roll back a
	// healthy, Running instance: expunging working compute over a non-essential,
	// write-only attribute is worse than a missing tag. Failures are surfaced as
	// warnings and the instance is kept and saved to state.
	if tags, tdiags := mapToStringMap(ctx, model.Tags); tdiags.HasError() {
		for _, d := range tdiags {
			resp.Diagnostics.AddWarning("Instance tags not applied", d.Detail())
		}
	} else {
		for k, v := range tags {
			if _, err := r.svc.CreateTag(ctx, slug, instance.TagRequest{Key: k, Value: v}); err != nil {
				resp.Diagnostics.AddWarning(
					"Instance tag not applied",
					fmt.Sprintf("tag %q on instance %s failed: %s. The instance was created; re-apply to set the tag.", k, slug, err.Error()),
				)
			}
		}
	}

	r.applyVMState(&model, ready)
	r.fillPublicIP(ctx, &model, ready)
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
	r.fillPublicIP(ctx, &model, vm)
	// tags are not refreshed (the API does not return them); preserved from state.
	// cloud_provider, region, template, plan, billing_cycle, project, ssh_key,
	// network_plan, storage_category, user_data are create/update inputs preserved
	// from state (not reliably echoed by the API in a comparable form).
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// Update applies the in-place changes in sequence (rename, startup script,
// resize, tags). Each step is idempotent. If a later step fails, the method
// returns without saving state, so the prior state is retained and Terraform
// re-runs the update on the next apply; the already-applied steps (e.g. a rename)
// simply re-run as no-ops. This is deliberate: persisting the plan on a partial
// failure could record a not-yet-applied step (e.g. a failed resize) as done and
// mask the drift.
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

	// 1. Display name (change-label). hostname is set once at create and never
	//    changed — only the display name is mutable.
	if !plan.Name.Equal(state.Name) {
		if err := r.svc.ChangeLabel(ctx, slug, plan.Name.ValueString()); err != nil {
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

	// 3. Plan / billing cycle (resize) — Terraform does not manage power state, but
	//    changing a CloudStack compute offering requires the VM to be stopped, so
	//    the provider transparently stops it (only if it was running), changes the
	//    offering, and restarts it back to its prior state. This is the only place
	//    power is touched, and it mirrors how aws_instance handles instance_type.
	if !plan.Plan.Equal(state.Plan) || !plan.BillingCycle.Equal(state.BillingCycle) {
		if err := r.resize(ctx, slug, plan); err != nil {
			resp.Diagnostics.AddError("Failed to resize instance", err.Error())
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

	// Re-read for consistent computed values (state, IPs).
	vm, err := r.svc.Get(ctx, slug)
	if err != nil {
		resp.Diagnostics.AddError("Failed to refresh instance after update", err.Error())
		return
	}
	r.applyVMState(&plan, vm)
	r.fillPublicIP(ctx, &plan, vm)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// waitForRunning polls the instance until it reports Running (returning it) or a
// terminal provisioning state. The state comes from the /meta endpoint, which
// forces a live reconcile against the hypervisor and is authoritative; the
// cached Get/List state can keep reporting "Starting" for many minutes after
// the VM is actually up (the CMP's background reconciliation is unreliable).
// The VM.CREATE activity log is still checked so a failed provisioning job
// errors out in seconds instead of blocking for the full create timeout.
func (r *instanceResource) waitForRunning(ctx context.Context, slug string) (*instance.VirtualMachine, error) {
	err := pollUntilReady(ctx, 10*time.Second, func(ctx context.Context) (bool, error) {
		if logs, lerr := r.svc.ActivityLogs(ctx, slug); lerr == nil {
			switch strings.ToUpper(createLogStatus(logs)) {
			case "FAILED", "ERROR":
				return false, fmt.Errorf("instance provisioning failed (VM.CREATE activity log status: %s)", createLogStatus(logs))
			}
		}
		meta, err := r.svc.Meta(ctx, slug)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(meta.State) {
		case "running":
			return true, nil
		case "error", "failed", "destroyed", "expunging", "expunged":
			return false, fmt.Errorf("instance entered state %q", meta.State)
		default:
			return false, nil
		}
	})
	if err != nil {
		return nil, err
	}
	// Re-read the full object for the computed fields. Calling /meta also
	// reconciled the stored state, but Get can still be momentarily stale, so
	// the authoritative state is pinned. The platform records the private IP
	// shortly after Running, so wait briefly for it to land in state at apply
	// time instead of on the next refresh. Best effort: a create is never
	// failed over a missing address.
	var vm *instance.VirtualMachine
	ipCtx, cancel := context.WithTimeout(ctx, privateIPPollWindow)
	defer cancel()
	_ = pollUntilReady(ipCtx, privateIPPollInterval, func(ctx context.Context) (bool, error) {
		v, gerr := r.svc.Get(ctx, slug)
		if gerr != nil {
			return false, gerr
		}
		vm = v
		return v.NetworkPrivateIP() != "", nil
	})
	if vm == nil {
		v, gerr := r.svc.Get(ctx, slug)
		if gerr != nil {
			return nil, gerr
		}
		vm = v
	}
	vm.State = "Running"
	return vm, nil
}

// createLogStatus returns the status of the VM.CREATE activity-log entry, or ""
// if there is no such entry yet.
func createLogStatus(logs []instance.ActivityLog) string {
	for _, l := range logs {
		if strings.EqualFold(l.Action, "VM.CREATE") {
			return l.Status
		}
	}
	return ""
}

// latestAction returns the status and created-at timestamp of the most recent
// VM.ACTION activity-log entry — the result of a stop/start/reboot job. Logs are
// newest-first, so the first match is the latest. Returns ("","") if none.
func latestAction(logs []instance.ActivityLog) (status, createdAt string) {
	for _, l := range logs {
		if strings.EqualFold(l.Action, "VM.ACTION") {
			return l.Status, l.CreatedAt
		}
	}
	return "", ""
}

// latestActionTime returns the created-at of the newest VM.ACTION log, used as a
// baseline captured before issuing a stop/start so a stale prior action is not
// mistaken for the new one. Best effort: "" if the log is unavailable.
func (r *instanceResource) latestActionTime(ctx context.Context, slug string) string {
	logs, err := r.svc.ActivityLogs(ctx, slug)
	if err != nil {
		return ""
	}
	_, at := latestAction(logs)
	return at
}

// waitForPowerState waits until the instance reaches targetState ("Stopped" or
// "Running") after a stop/start. The state comes from the /meta endpoint,
// which reconciles live against the hypervisor; the cached Get state can lag
// many minutes behind a power change. The VM.ACTION activity log is still
// checked for a failed job, which /meta cannot signal (a failed stop just
// keeps reporting the old state until the timeout). baselineAction is the
// newest VM.ACTION timestamp from before the operation was issued, so a stale
// prior action is never mistaken for the new one.
func (r *instanceResource) waitForPowerState(ctx context.Context, slug, targetState, baselineAction string) error {
	return pollUntilReady(ctx, 10*time.Second, func(ctx context.Context) (bool, error) {
		if logs, lerr := r.svc.ActivityLogs(ctx, slug); lerr == nil {
			if status, at := latestAction(logs); at != "" && at != baselineAction {
				switch strings.ToUpper(status) {
				case "FAILED", "ERROR":
					return false, fmt.Errorf("instance failed to reach %s (VM.ACTION status: %s)", targetState, status)
				}
			}
		}
		meta, gerr := r.svc.Meta(ctx, slug)
		if gerr != nil {
			return false, gerr
		}
		return strings.EqualFold(meta.State, targetState), nil
	})
}

// cleanupAfterFailedCreate best-effort deletes an instance that was provisioned
// but cannot be saved to state because a later create step failed. A cleanup
// failure is surfaced as a warning (the original error is reported by the caller)
// so the user knows a manual delete may be needed.
func (r *instanceResource) cleanupAfterFailedCreate(ctx context.Context, slug string, deletePublicIP bool, diags *diag.Diagnostics) {
	if err := r.svc.Delete(ctx, slug, true, deletePublicIP); err != nil && !apierrors.IsNotFound(err) && !apierrors.IsResourceNotFound(err) {
		diags.AddWarning(
			"Orphaned instance not cleaned up",
			fmt.Sprintf("instance %s was created but provisioning failed, and the cleanup delete also failed: %s. Delete it manually to avoid an orphan.", slug, err.Error()),
		)
	}
}

// resize changes the instance's compute offering. Because CloudStack requires a
// stopped VM to change its offering, it stops the VM first if it is running and
// restarts it afterward, leaving it in the same power state it started in. If the
// offering change fails after the VM was stopped, the VM is restarted (when it
// was running) before the error is returned, preserving the transparent-resize
// contract that a failed resize does not leave a previously-running VM stopped.
func (r *instanceResource) resize(ctx context.Context, slug string, plan instanceResourceModel) error {
	vm, err := r.svc.Get(ctx, slug)
	if err != nil {
		return err
	}
	wasRunning := isRunning(vm.State)

	if wasRunning {
		base := r.latestActionTime(ctx, slug)
		if _, err := r.svc.Stop(ctx, slug); err != nil {
			return err
		}
		if err := r.waitForPowerState(ctx, slug, "Stopped", base); err != nil {
			return err
		}
	}

	changeErr := r.changeOffering(ctx, slug, plan)
	if changeErr != nil {
		// Restore the prior power state before surfacing the error.
		if wasRunning {
			base := r.latestActionTime(ctx, slug)
			if _, serr := r.svc.Start(ctx, slug); serr == nil {
				_ = r.waitForPowerState(ctx, slug, "Running", base)
			}
		}
		return changeErr
	}

	if wasRunning {
		base := r.latestActionTime(ctx, slug)
		if _, err := r.svc.Start(ctx, slug); err != nil {
			return err
		}
		if err := r.waitForPowerState(ctx, slug, "Running", base); err != nil {
			return err
		}
	}
	return nil
}

// changeOffering performs the ChangePlan call and waits for the VM to settle in
// Stopped (where a successful offering change leaves it).
func (r *instanceResource) changeOffering(ctx context.Context, slug string, plan instanceResourceModel) error {
	base := r.latestActionTime(ctx, slug)
	if _, err := r.svc.ChangePlan(ctx, slug, instance.ChangePlanRequest{
		Plan:         plan.Plan.ValueString(),
		Slug:         slug,
		VM:           slug,
		BillingCycle: plan.BillingCycle.ValueString(),
	}); err != nil {
		return err
	}
	return r.waitForPowerState(ctx, slug, "Stopped", base)
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
	// delete_public_ip asks the API to release the IP it auto-assigned at create
	// so destroy does not strand a billed address. The live API currently ignores
	// the flag (the CMP IP-release endpoint rejects token auth, a known
	// platform bug with a fix in progress), so the IP stays Allocated until it is
	// released manually. The flag is still sent so destroy heals automatically
	// once the platform fix lands. Only set when this resource requested the
	// auto-assignment (assign_public_ip defaults to true, mirroring Create). An IP
	// attached via zcp_ip_address/zcp_ip_association is owned by those resources.
	deletePublicIP := model.AssignPublicIP.IsNull() || model.AssignPublicIP.IsUnknown() || model.AssignPublicIP.ValueBool()
	// expunge=true forces an immediate purge so the slug does not linger in a
	// soft-deleted state (which would otherwise make pollUntilGone time out).
	err := r.svc.Delete(deleteCtx, slug, true, deletePublicIP)
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
// <slug>/<cloud_provider>/<region>/<template> are required. name, state and IPs
// come from the subsequent Read; tags cannot be imported (the API does not
// return them).
func (r *instanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"id", "cloud_provider", "region", "template", "plan", "billing_cycle", "project", "ssh_key", "network", "network_plan", "storage_category"}
	importPositional(ctx, req, resp, fields, 4,
		"<slug>/<cloud_provider>/<region>/<template>[/<plan>/<billing_cycle>/<project>/<ssh_key>/<network>/<network_plan>/<storage_category>]")
}
