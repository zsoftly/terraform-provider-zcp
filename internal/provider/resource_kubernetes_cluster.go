package provider

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/kubernetes"
)

var _ resource.Resource = &kubernetesClusterResource{}
var _ resource.ResourceWithConfigure = &kubernetesClusterResource{}
var _ resource.ResourceWithImportState = &kubernetesClusterResource{}

type kubernetesServiceIface interface {
	Create(ctx context.Context, req kubernetes.CreateRequest) (*kubernetes.Cluster, error)
	Get(ctx context.Context, slug string) (*kubernetes.Cluster, error)
	Scale(ctx context.Context, slug string, nodeSize int) error
	Delete(ctx context.Context, slug string) error
}

type kubernetesClusterResource struct {
	svc            kubernetesServiceIface
	defaultProject string
}

type kubernetesClusterResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	CloudProvider   types.String `tfsdk:"cloud_provider"`
	Region          types.String `tfsdk:"region"`
	Version         types.String `tfsdk:"version"`
	Plan            types.String `tfsdk:"plan"`
	BillingCycle    types.String `tfsdk:"billing_cycle"`
	Workers         types.Int64  `tfsdk:"workers"`
	StorageCategory types.String `tfsdk:"storage_category"`
	Project         types.String `tfsdk:"project"`
	SSHKey          types.String `tfsdk:"ssh_key"`
	ControlNodes    types.Int64  `tfsdk:"control_nodes"`
	HA              types.Bool   `tfsdk:"ha"`
	// Computed
	Slug        types.String   `tfsdk:"slug"`
	State       types.String   `tfsdk:"state"`
	APIEndpoint types.String   `tfsdk:"api_endpoint"`
	IPAddress   types.String   `tfsdk:"ip_address"`
	Timeouts    timeouts.Value `tfsdk:"timeouts"`
}

func NewKubernetesClusterResource() resource.Resource {
	return &kubernetesClusterResource{}
}

func (r *kubernetesClusterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kubernetes_cluster"
}

func (r *kubernetesClusterResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP managed Kubernetes cluster. Create blocks until the cluster reaches the `Running` state; changing `workers` scales the cluster in place.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Cluster slug (unique identifier).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name for the cluster. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"cloud_provider": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cloud provider slug. Use `data.zcp_region.<name>.cloud_provider` instead of hardcoding. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Region slug where the cluster is created (e.g. `yow-1`). Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"version": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Kubernetes version (e.g. `v1.36.1`). Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"plan": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cluster node plan slug (e.g. `k8s-li-yow-1`). Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"billing_cycle": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Billing cycle (`hourly` or `monthly`). Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"workers": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Number of worker nodes (>= 1). Changing this scales the cluster in place.",
				Validators:          []validator.Int64{int64AtLeastValidator{min: 1}},
			},
			"storage_category": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Storage category slug. Region-specific: `nvme` in yow-1, `pro-nvme`/`premium-ssd` in yul-1. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project slug. Inherits from the provider `default_project` if omitted. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"ssh_key": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Name of an existing SSH key for node login (see `zcp_ssh_key`). Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"control_nodes": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Number of control-plane nodes (default 1; use >= 3 for HA). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace(), int64planmodifier.UseStateForUnknown()},
				Validators:          []validator.Int64{int64AtLeastValidator{min: 1}},
			},
			"ha": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Enable high availability. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace(), boolplanmodifier.UseStateForUnknown()},
			},
			"slug": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Cluster slug (same value as `id`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current cluster state (e.g. `Running`).",
			},
			"api_endpoint": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Kubernetes API server endpoint, populated once the cluster is Running.",
			},
			"ip_address": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Public IP address of the cluster, populated once the cluster is Running.",
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

func (r *kubernetesClusterResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = kubernetes.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

// clusterWorkers returns the authoritative worker count, preferring the
// CloudStack-side meta size (populated once Running) over node_size.
func clusterWorkers(c *kubernetes.Cluster) int64 {
	if c.Meta != nil && c.Meta.Size != "" {
		if n, err := strconv.Atoi(c.Meta.Size); err == nil {
			return int64(n)
		}
	}
	return int64(c.NodeSize)
}

// applyClusterState populates the unambiguously-computed attributes (id, slug,
// state, endpoints) from a cluster object.
//
// It deliberately does NOT touch the node-count/ha attributes (`workers`,
// `control_nodes`, `ha`): the API reports transient 0/stale counts immediately
// after create or scale, and those attributes are either Required or carry a
// user-configured value that is authoritative. Overwriting them from the API
// readback here would violate Terraform's "consistent result after apply"
// contract. Each method sets them from the plan (Create/Update) or reconciles
// them in Read, guarded against the transient value.
func (r *kubernetesClusterResource) applyClusterState(model *kubernetesClusterResourceModel, c *kubernetes.Cluster) {
	model.ID = types.StringValue(c.Slug)
	model.Slug = types.StringValue(c.Slug)
	model.State = types.StringValue(c.State)

	endpoint := ""
	ip := derefString(c.PublicIP)
	if c.Meta != nil {
		endpoint = c.Meta.Endpoint
		if c.Meta.IPAddress != "" {
			ip = c.Meta.IPAddress
		}
	}
	model.APIEndpoint = types.StringValue(endpoint)
	model.IPAddress = types.StringValue(ip)
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (r *kubernetesClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model kubernetesClusterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_kubernetes_cluster cannot be created: bearer_token is missing.")
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

	workers := int(model.Workers.ValueInt64())
	controlNodes := 1
	if !model.ControlNodes.IsNull() && !model.ControlNodes.IsUnknown() {
		controlNodes = int(model.ControlNodes.ValueInt64())
	}

	createReq := kubernetes.CreateRequest{
		Name:            model.Name.ValueString(),
		Version:         model.Version.ValueString(),
		NodeSize:        workers,
		WorkerNodeSize:  workers,
		ControlNodes:    controlNodes,
		CloudProvider:   model.CloudProvider.ValueString(),
		Region:          model.Region.ValueString(),
		Project:         project,
		BillingCycle:    model.BillingCycle.ValueString(),
		EnableHA:        model.HA.ValueBool(),
		Networks:        []string{},
		Plan:            model.Plan.ValueString(),
		StorageCategory: model.StorageCategory.ValueString(),
	}
	if sshKey := model.SSHKey.ValueString(); sshKey != "" {
		createReq.SSHKey = sshKey
		createReq.AuthMethod = "ssh-key"
	}

	cluster, err := r.svc.Create(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create Kubernetes cluster", err.Error())
		return
	}
	slug := cluster.Slug

	ready, err := r.waitForRunning(ctx, slug)
	if err != nil {
		resp.Diagnostics.AddError(
			"Cluster did not reach Running",
			fmt.Sprintf("cluster %s was created but did not become Running: %s", slug, err.Error()),
		)
		return
	}

	r.applyClusterState(&model, ready)
	// Resolve the Optional+Computed node-count/ha attributes from the values we
	// actually sent (authoritative), not the API's transient post-create
	// readback. `workers` is already the plan value in model.
	model.ControlNodes = types.Int64Value(int64(controlNodes))
	model.HA = types.BoolValue(model.HA.ValueBool())
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// waitForRunning polls until the cluster reports Running (returning it) or a
// terminal failure state.
func (r *kubernetesClusterResource) waitForRunning(ctx context.Context, slug string) (*kubernetes.Cluster, error) {
	var latest *kubernetes.Cluster
	err := pollUntilReady(ctx, 15*time.Second, func(ctx context.Context) (bool, error) {
		c, err := r.svc.Get(ctx, slug)
		if err != nil {
			return false, err
		}
		latest = c
		switch c.State {
		case "Running":
			return true, nil
		case "Error", "Failed":
			return false, fmt.Errorf("cluster entered state %q", c.State)
		default:
			return false, nil
		}
	})
	if err != nil {
		return nil, err
	}
	return latest, nil
}

// waitForScale polls until the cluster is Running AND reports the desired worker
// count, returning the cluster. A scale request leaves the cluster momentarily
// in Running (with the old count) before it transitions Running -> Scaling ->
// Running, so waiting on the count — not merely on Running — is required to
// confirm the scale actually took effect (and to leave the cluster in a stable,
// deletable state). The first interval is skipped via pollUntilReady's immediate
// check only when the count already matches.
func (r *kubernetesClusterResource) waitForScale(ctx context.Context, slug string, desired int64) (*kubernetes.Cluster, error) {
	var latest *kubernetes.Cluster
	err := pollUntilReady(ctx, 15*time.Second, func(ctx context.Context) (bool, error) {
		c, err := r.svc.Get(ctx, slug)
		if err != nil {
			return false, err
		}
		latest = c
		if c.State == "Error" || c.State == "Failed" {
			return false, fmt.Errorf("cluster entered state %q", c.State)
		}
		return c.State == "Running" && clusterWorkers(c) == desired, nil
	})
	if err != nil {
		return nil, err
	}
	return latest, nil
}

func (r *kubernetesClusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model kubernetesClusterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_kubernetes_cluster cannot be read: bearer_token is missing.")
		return
	}

	c, err := r.svc.Get(ctx, model.ID.ValueString())
	if err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read Kubernetes cluster", err.Error())
		return
	}

	model.Name = types.StringValue(c.Name)
	r.applyClusterState(&model, c)
	// Reconcile the node-count/ha attributes from the API, but only trust a sane
	// (>0) count — the API briefly reports 0 right after create/scale, and
	// writing that would force a spurious replace/diff.
	if w := clusterWorkers(c); w > 0 {
		model.Workers = types.Int64Value(w)
	}
	if c.ControlNodes > 0 {
		model.ControlNodes = types.Int64Value(int64(c.ControlNodes))
	}
	// ha is create-only (RequiresReplace). The API may report it differently from
	// the submitted value (e.g. HA implicitly enabled for control_nodes >= 3),
	// which would otherwise produce a perpetual forced-replace diff, so preserve
	// the configured value and only adopt the API value when state has none (import).
	if model.HA.IsNull() || model.HA.IsUnknown() {
		model.HA = types.BoolValue(c.EnableHA)
	}
	// version, plan, billing_cycle, storage_category, cloud_provider, project and
	// ssh_key are create-only; preserved from state to avoid format-driven diffs.
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// Update scales the worker count in place. `workers` is the only mutable
// attribute (everything else is RequiresReplace).
func (r *kubernetesClusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model kubernetesClusterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state kubernetesClusterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_kubernetes_cluster cannot be updated: bearer_token is missing.")
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
	desired := int(model.Workers.ValueInt64())

	if desired != int(state.Workers.ValueInt64()) {
		if err := r.svc.Scale(ctx, slug, desired); err != nil {
			resp.Diagnostics.AddError("Failed to scale Kubernetes cluster", err.Error())
			return
		}
		// Block until the cluster is Running AND actually reports the desired
		// worker count — a freshly-requested scale leaves it momentarily Running
		// with the old count before transitioning Running -> Scaling -> Running.
		// Returning early would both mis-report the count and leave a Scaling
		// cluster that the delete API silently refuses to process.
		ready, err := r.waitForScale(ctx, slug, int64(desired))
		if err != nil {
			resp.Diagnostics.AddError("Cluster scaling did not complete", err.Error())
			return
		}
		r.applyClusterState(&model, ready)
		// workers is the desired plan value (authoritative — the API may still
		// report the pre-scale count); control_nodes and ha are immutable.
		model.ControlNodes = state.ControlNodes
		model.HA = state.HA
		resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
		return
	}

	// No worker change — preserve computed values from state.
	model.ID = state.ID
	model.Slug = state.Slug
	model.State = state.State
	model.APIEndpoint = state.APIEndpoint
	model.IPAddress = state.IPAddress
	model.ControlNodes = state.ControlNodes
	model.HA = state.HA
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *kubernetesClusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model kubernetesClusterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_kubernetes_cluster cannot be deleted: bearer_token is missing.")
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
	if err != nil && !apierrors.IsNotFound(err) && !apierrors.IsResourceNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete Kubernetes cluster", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 15*time.Second, func(ctx context.Context) (bool, error) {
		_, err := r.svc.Get(ctx, slug)
		if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
			return false, nil
		}
		return err == nil, err
	}); err != nil {
		resp.Diagnostics.AddError("Cluster deletion did not complete", err.Error())
	}
}

// ImportState seeds the create-only attributes the API does not return in a
// comparable form so a post-import plan is zero-diff. Format (slash-separated,
// trailing/empty segments allowed):
//
//	<slug>/<cloud_provider>/<region>/<version>/<plan>/<billing_cycle>/<storage_category>/<project>/<ssh_key>
//
// <slug>/<cloud_provider>/<region>/<version>/<plan>/<billing_cycle>/<storage_category>
// are required. workers, control_nodes, ha, name, state and endpoints come from
// the subsequent Read.
func (r *kubernetesClusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"id", "cloud_provider", "region", "version", "plan", "billing_cycle", "storage_category", "project", "ssh_key"}
	importPositional(ctx, req, resp, fields, 7,
		"<slug>/<cloud_provider>/<region>/<version>/<plan>/<billing_cycle>/<storage_category>[/<project>/<ssh_key>]")
}
