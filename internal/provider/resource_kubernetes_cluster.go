package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/billing"
	"github.com/zsoftly/zcp-cli/pkg/api/kubernetes"
	"github.com/zsoftly/zcp-cli/pkg/httpclient"
)

var _ resource.Resource = &kubernetesClusterResource{}
var _ resource.ResourceWithConfigure = &kubernetesClusterResource{}
var _ resource.ResourceWithImportState = &kubernetesClusterResource{}

var kubernetesPollInterval = 15 * time.Second

type kubernetesServiceIface interface {
	Create(ctx context.Context, req kubernetes.CreateRequest) (*kubernetes.Cluster, error)
	Get(ctx context.Context, slug string) (*kubernetes.Cluster, error)
	ListVersions(ctx context.Context) ([]kubernetes.KubernetesVersion, error)
	Scale(ctx context.Context, slug string, nodeSize int) error
	EnableAutoscaling(ctx context.Context, slug string, minWorkers, maxWorkers int) error
	DisableAutoscaling(ctx context.Context, slug string, workers int) error
	Upgrade(ctx context.Context, slug string, req kubernetes.UpgradeRequest) error
	UpgradeVersion(ctx context.Context, clusterSlug string, req kubernetes.UpgradeVersionRequest) error
	Delete(ctx context.Context, slug string) error
	Cancel(ctx context.Context, cluster *kubernetes.Cluster) error
}

type kubernetesService struct {
	*kubernetes.Service
	client *httpclient.Client
}

func (s *kubernetesService) Cancel(ctx context.Context, cluster *kubernetes.Cluster) error {
	deletePublicIP := true
	cycle := ""
	cycles := []*kubernetes.BillingCycle{nil, cluster.BillingCycle}
	if cluster.Offering != nil {
		cycles[0] = cluster.Offering.BillingCycle
	}
	for _, candidate := range cycles {
		if candidate == nil {
			continue
		}
		for _, value := range []string{candidate.Unit, candidate.Slug, candidate.Name} {
			if strings.HasPrefix(strings.ToLower(value), "hour") {
				cycle = "hour"
				break
			}
			if strings.HasPrefix(strings.ToLower(value), "month") {
				cycle = "month"
				break
			}
		}
		if cycle != "" {
			break
		}
	}
	return billing.NewService(s.client).CancelService(ctx, cluster.Slug, billing.CancelServiceRequest{ServiceName: "Kubernetes", Reason: "not_needed_anymore", Type: "Immediate", Status: "Pending", BillingCycle: cycle, DeletePublicIP: &deletePublicIP})
}

type kubernetesClusterResource struct {
	svc            kubernetesServiceIface
	defaultProject string
}

type kubernetesClusterResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	CloudProvider types.String `tfsdk:"cloud_provider"`
	Region        types.String `tfsdk:"region"`
	Version       types.String `tfsdk:"version"`
	// Plan is retained only to decode and destroy state created by provider
	// releases before the separate Kubernetes node plans were introduced.
	Plan             types.String `tfsdk:"plan"`
	ControlPlanePlan types.String `tfsdk:"control_plane_plan"`
	WorkerPlan       types.String `tfsdk:"worker_plan"`
	StoragePlan      types.String `tfsdk:"storage_plan"`
	RootDiskSize     types.Int64  `tfsdk:"root_disk_size"`
	BillingCycle     types.String `tfsdk:"billing_cycle"`
	Workers          types.Int64  `tfsdk:"workers"`
	StorageCategory  types.String `tfsdk:"storage_category"`
	Project          types.String `tfsdk:"project"`
	SSHKey           types.String `tfsdk:"ssh_key"`
	ControlNodes     types.Int64  `tfsdk:"control_nodes"`
	HA               types.Bool   `tfsdk:"ha"`
	EnableCSI        types.Bool   `tfsdk:"enable_csi"`
	Autoscaling      types.Bool   `tfsdk:"autoscaling"`
	MinWorkers       types.Int64  `tfsdk:"min_workers"`
	MaxWorkers       types.Int64  `tfsdk:"max_workers"`
	// Computed
	Slug            types.String   `tfsdk:"slug"`
	State           types.String   `tfsdk:"state"`
	APIEndpoint     types.String   `tfsdk:"api_endpoint"`
	IPAddress       types.String   `tfsdk:"ip_address"`
	NetworkName     types.String   `tfsdk:"network_name"`
	NetworkSlug     types.String   `tfsdk:"network_slug"`
	RootVolumeSlugs types.List     `tfsdk:"root_volume_slugs"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
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
		MarkdownDescription: "Manages a ZCP Kubernetes cluster using fixed control-plane, worker, and root-volume plans. Create blocks until the cluster reaches the `Running` state.",
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
				MarkdownDescription: "Kubernetes version string (e.g. `v1.36.4`). Changing this upgrades the cluster in place.",
			},
			"plan": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Legacy single-plan field retained for state created by earlier provider releases. New clusters use `control_plane_plan` and `worker_plan`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"control_plane_plan": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Fixed control-plane node plan slug. Required for a new cluster. Changing this forces replacement.", PlanModifiers: requiresReplace},
			"worker_plan":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Fixed worker node plan slug. Required for a new cluster. Changing this forces replacement.", PlanModifiers: requiresReplace},
			"storage_plan":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Fixed root-volume storage tier slug. `custom_plan` is not supported. Required for a new cluster. Changing this forces replacement.", PlanModifiers: requiresReplace},
			"root_disk_size":     schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Root-volume capacity in GB for the selected storage tier. Required for a new cluster. Changing this forces replacement.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}, Validators: []validator.Int64{int64AtLeastValidator{min: 1}}},
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
				MarkdownDescription: "Number of control-plane nodes (default 1; use >= 2 for HA). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown(), int64planmodifier.RequiresReplace()},
				Validators:          []validator.Int64{int64AtLeastValidator{min: 1}},
			},
			"ha": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Enable high availability. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown(), boolplanmodifier.RequiresReplace()},
			},
			"enable_csi":  schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable Cloud Storage Integration (CSI) at creation. Changing this forces replacement.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown(), boolplanmodifier.RequiresReplace()}},
			"autoscaling": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable worker autoscaling. Enabling records the requested bounds; it does not wait for workers to scale out.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"min_workers": schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Minimum worker count when `autoscaling` is true.", PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}, Validators: []validator.Int64{int64AtLeastValidator{min: 1}}},
			"max_workers": schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Maximum worker count when `autoscaling` is true.", PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}, Validators: []validator.Int64{int64AtLeastValidator{min: 1}}},
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
			"network_name":      schema.StringAttribute{Computed: true, MarkdownDescription: "Attached network name, when returned by the API."},
			"network_slug":      schema.StringAttribute{Computed: true, MarkdownDescription: "Attached network slug, when returned by the API."},
			"root_volume_slugs": schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Root-volume slugs returned by the API."},
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
	r.svc = &kubernetesService{Service: kubernetes.NewService(pd.Client), client: pd.Client}
	r.defaultProject = pd.DefaultProject
}

// clusterWorkers returns the authoritative worker count. It uses the platform
// metadata size, when available, before node_size.
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
	if c.Network != nil {
		model.NetworkName = types.StringValue(c.Network.Name)
		model.NetworkSlug = types.StringValue(c.Network.Slug)
	} else {
		model.NetworkName = types.StringValue("")
		model.NetworkSlug = types.StringValue("")
	}
	rootVolumes := make([]string, 0)
	for _, volume := range c.BlockStorages {
		if volume.IsRoot {
			if volume.Slug != "" {
				rootVolumes = append(rootVolumes, volume.Slug)
			}
		}
	}
	model.RootVolumeSlugs = types.ListValueMust(types.StringType, stringValues(rootVolumes))
	if enabled, min, max, known := clusterAutoscaling(c); known {
		model.Autoscaling = types.BoolValue(enabled)
		if min > 0 {
			model.MinWorkers = types.Int64Value(min)
		}
		if max > 0 {
			model.MaxWorkers = types.Int64Value(max)
		}
	}
}

func normalizeKubernetesUpdateState(model, prior *kubernetesClusterResourceModel) {
	if model.Plan.IsUnknown() {
		model.Plan = prior.Plan
		if model.Plan.IsUnknown() {
			model.Plan = types.StringNull()
		}
	}
	if model.ControlPlanePlan.IsUnknown() {
		model.ControlPlanePlan = prior.ControlPlanePlan
	}
	if model.WorkerPlan.IsUnknown() {
		model.WorkerPlan = prior.WorkerPlan
	}
	if model.StoragePlan.IsUnknown() {
		model.StoragePlan = prior.StoragePlan
	}
	if model.RootDiskSize.IsUnknown() {
		model.RootDiskSize = prior.RootDiskSize
	}
	if model.ControlNodes.IsUnknown() {
		model.ControlNodes = prior.ControlNodes
	}
	if model.HA.IsUnknown() {
		model.HA = prior.HA
	}
	if model.EnableCSI.IsUnknown() {
		model.EnableCSI = prior.EnableCSI
	}
	if model.Autoscaling.IsUnknown() {
		model.Autoscaling = prior.Autoscaling
	}
	if model.MinWorkers.IsUnknown() {
		model.MinWorkers = prior.MinWorkers
	}
	if model.MaxWorkers.IsUnknown() {
		model.MaxWorkers = prior.MaxWorkers
	}
}

func stringValues(values []string) []attr.Value {
	result := make([]attr.Value, len(values))
	for i, value := range values {
		result[i] = types.StringValue(value)
	}
	return result
}

func rawJSONValue(raw json.RawMessage) string {
	var value interface{}
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func clusterAutoscaling(c *kubernetes.Cluster) (enabled bool, min, max int64, known bool) {
	if c.Autoscale != nil {
		enabled, known = true, true
	}
	if c.Meta == nil {
		return
	}
	if len(c.Meta.AutoscalingEnabled) > 0 && string(c.Meta.AutoscalingEnabled) != "null" {
		known = true
		switch strings.ToLower(rawJSONValue(c.Meta.AutoscalingEnabled)) {
		case "", "0", "false", "no":
			enabled = false
		default:
			enabled = true
		}
	}
	parse := func(raw json.RawMessage) int64 {
		n, err := strconv.ParseInt(rawJSONValue(raw), 10, 64)
		if err != nil {
			return 0
		}
		return n
	}
	min, max = parse(c.Meta.MinSize), parse(c.Meta.MaxSize)
	return
}

func validateKubernetesCreate(model *kubernetesClusterResourceModel) error {
	if !model.Plan.IsNull() && !model.Plan.IsUnknown() && model.Plan.ValueString() != "" {
		return fmt.Errorf("plan is a legacy state field; use control_plane_plan and worker_plan for a new cluster")
	}
	if model.ControlPlanePlan.IsNull() || model.ControlPlanePlan.IsUnknown() || model.WorkerPlan.IsNull() || model.WorkerPlan.IsUnknown() || model.StoragePlan.IsNull() || model.StoragePlan.IsUnknown() || model.RootDiskSize.IsNull() || model.RootDiskSize.IsUnknown() {
		return fmt.Errorf("fixed plans and root_disk_size must be known before creating a Kubernetes cluster")
	}
	if model.ControlPlanePlan.ValueString() == "" {
		return fmt.Errorf("control_plane_plan must name a control-plane plan before creating a Kubernetes cluster")
	}
	if model.WorkerPlan.ValueString() == "" {
		return fmt.Errorf("worker_plan must name a worker plan before creating a Kubernetes cluster")
	}
	if model.StoragePlan.ValueString() == "" || model.StoragePlan.ValueString() == "custom_plan" {
		return fmt.Errorf("storage_plan must name a storage tier; custom_plan is not supported")
	}
	if model.HA.ValueBool() && model.ControlNodes.ValueInt64() < 2 {
		return fmt.Errorf("control_nodes must be at least 2 when ha is true")
	}
	return validateKubernetesAutoscaling(model)
}

func validateKubernetesAutoscaling(model *kubernetesClusterResourceModel) error {
	if model.Autoscaling.IsNull() || model.Autoscaling.IsUnknown() || !model.Autoscaling.ValueBool() {
		// UseStateForUnknown keeps prior bounds during a disable transition. They
		// are cleared from state after the API confirms autoscaling is off.
		return nil
	}
	if model.MinWorkers.IsNull() || model.MinWorkers.IsUnknown() || model.MaxWorkers.IsNull() || model.MaxWorkers.IsUnknown() {
		return fmt.Errorf("min_workers and max_workers are required when autoscaling is true")
	}
	if model.MaxWorkers.ValueInt64() < model.MinWorkers.ValueInt64() {
		return fmt.Errorf("max_workers must be at least min_workers when autoscaling is true")
	}
	return nil
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
	if err := validateKubernetesCreate(&model); err != nil {
		resp.Diagnostics.AddError("Invalid Kubernetes cluster configuration", err.Error())
		return
	}
	if model.EnableCSI.IsNull() || model.EnableCSI.IsUnknown() {
		model.EnableCSI = types.BoolValue(false)
	}
	if model.Autoscaling.IsNull() || model.Autoscaling.IsUnknown() {
		model.Autoscaling = types.BoolValue(false)
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

	fixedPlan := false
	createReq := kubernetes.CreateRequest{
		Name:                   model.Name.ValueString(),
		Version:                model.Version.ValueString(),
		NodeSize:               workers,
		WorkerNodeSize:         workers,
		ControlNodes:           controlNodes,
		CloudProvider:          model.CloudProvider.ValueString(),
		Region:                 model.Region.ValueString(),
		Project:                project,
		BillingCycle:           model.BillingCycle.ValueString(),
		EnableHA:               model.HA.ValueBool(),
		EnableCSI:              model.EnableCSI.ValueBool(),
		Networks:               []string{},
		MasterPlan:             model.ControlPlanePlan.ValueString(),
		WorkerPlan:             model.WorkerPlan.ValueString(),
		BlockstoragePlan:       model.StoragePlan.ValueString(),
		BlockstorageCustomPlan: &kubernetes.BlockstorageCustomPlan{Storage: int(model.RootDiskSize.ValueInt64())},
		IsK8sCustomPlan:        &fixedPlan,
		MasterCustomPlan:       json.RawMessage("null"),
		IsK8sMasterCustomPlan:  &fixedPlan,
		WorkerCustomPlan:       json.RawMessage("null"),
		IsK8sWorkerCustomPlan:  &fixedPlan,
		WithPoolCard:           false,
		IsCustomPlan:           false,
		CustomPlan:             nil,
		VirtualMachine:         "",
		StorageCategory:        model.StorageCategory.ValueString(),
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
		r.applyClusterState(&model, cluster)
		resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
		resp.Diagnostics.AddError(
			"Cluster did not reach Running",
			fmt.Sprintf("cluster %s was created but did not become Running: %s", slug, err.Error()),
		)
		return
	}

	desiredAutoscaling := model.Autoscaling.ValueBool()
	desiredMinWorkers, desiredMaxWorkers := model.MinWorkers, model.MaxWorkers
	r.applyClusterState(&model, ready)
	// Resolve the Optional+Computed node-count/ha attributes from the values we
	// actually sent (authoritative), not the API's transient post-create
	// readback. `workers` is already the plan value in model.
	model.ControlNodes = types.Int64Value(int64(controlNodes))
	model.HA = types.BoolValue(model.HA.ValueBool())
	model.EnableCSI = types.BoolValue(model.EnableCSI.ValueBool())
	if desiredAutoscaling {
		if err := r.svc.EnableAutoscaling(ctx, slug, int(desiredMinWorkers.ValueInt64()), int(desiredMaxWorkers.ValueInt64())); err != nil {
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			resp.Diagnostics.AddError("Failed to enable Kubernetes autoscaling", err.Error())
			return
		}
		configured, err := r.waitForAutoscaling(ctx, slug, true, desiredMinWorkers.ValueInt64(), desiredMaxWorkers.ValueInt64())
		if err != nil {
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			resp.Diagnostics.AddError("Kubernetes autoscaling configuration did not complete", err.Error())
			return
		}
		r.applyClusterState(&model, configured)
		// The platform scales out asynchronously. The configured bounds, not a
		// future worker count, are the completion condition for this operation.
		model.Autoscaling, model.MinWorkers, model.MaxWorkers = types.BoolValue(true), desiredMinWorkers, desiredMaxWorkers
	}
	if model.Plan.IsUnknown() {
		model.Plan = types.StringNull()
	}
	if model.MinWorkers.IsUnknown() {
		model.MinWorkers = types.Int64Null()
	}
	if model.MaxWorkers.IsUnknown() {
		model.MaxWorkers = types.Int64Null()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// waitForRunning polls until the cluster reports Running (returning it) or a
// terminal failure state.
func (r *kubernetesClusterResource) waitForRunning(ctx context.Context, slug string) (*kubernetes.Cluster, error) {
	var latest *kubernetes.Cluster
	err := pollUntilReady(ctx, kubernetesPollInterval, func(ctx context.Context) (bool, error) {
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
	err := pollUntilReady(ctx, kubernetesPollInterval, func(ctx context.Context) (bool, error) {
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

// waitForAutoscaling confirms the configuration accepted by the platform. It
// does not wait for a scale-out event: worker demand, not Terraform, decides
// whether and when autoscaling adds nodes.
func (r *kubernetesClusterResource) waitForAutoscaling(ctx context.Context, slug string, enabled bool, min, max int64) (*kubernetes.Cluster, error) {
	var latest *kubernetes.Cluster
	err := pollUntilReady(ctx, kubernetesPollInterval, func(ctx context.Context) (bool, error) {
		c, err := r.svc.Get(ctx, slug)
		if err != nil {
			return false, err
		}
		latest = c
		if c.State == "Error" || c.State == "Failed" {
			return false, fmt.Errorf("cluster entered state %q", c.State)
		}
		actualEnabled, actualMin, actualMax, known := clusterAutoscaling(c)
		if !known || actualEnabled != enabled {
			return false, nil
		}
		if enabled {
			return actualMin == min && actualMax == max, nil
		}
		return c.State == "Running", nil
	})
	if err != nil {
		return nil, err
	}
	return latest, nil
}

func clusterVersionMatches(c *kubernetes.Cluster, target kubernetes.KubernetesVersion) bool {
	if c.Version == target.Version || c.Version == target.Slug {
		return true
	}
	if c.Meta == nil {
		return false
	}
	return c.Meta.KubernetesVersionName == target.Version ||
		c.Meta.KubernetesVersionName == target.Name ||
		c.Meta.KubernetesVersionID == target.KubernetesClusterVersionID ||
		c.Meta.KubernetesVersionID == target.ID
}

func (r *kubernetesClusterResource) resolveVersionForCluster(ctx context.Context, c *kubernetes.Cluster, desired string) (kubernetes.KubernetesVersion, error) {
	versions, err := r.svc.ListVersions(ctx)
	if err != nil {
		return kubernetes.KubernetesVersion{}, err
	}

	regionID := c.RegionID
	if regionID == "" && c.Region != nil {
		regionID = c.Region.ID
	}

	var fallback *kubernetes.KubernetesVersion
	for i := range versions {
		v := versions[i]
		if v.Slug != desired && v.Version != desired {
			continue
		}
		if regionID == "" || v.RegionID == regionID {
			return v, nil
		}
		if fallback == nil {
			fallback = &v
		}
	}
	if fallback != nil {
		return *fallback, nil
	}
	return kubernetes.KubernetesVersion{}, fmt.Errorf("kubernetes version %q is not available for this cluster", desired)
}

func (r *kubernetesClusterResource) waitForVersion(ctx context.Context, slug string, target kubernetes.KubernetesVersion) (*kubernetes.Cluster, error) {
	var latest *kubernetes.Cluster
	err := pollUntilReady(ctx, kubernetesPollInterval, func(ctx context.Context) (bool, error) {
		c, err := r.svc.Get(ctx, slug)
		if err != nil {
			return false, err
		}
		latest = c
		if c.State == "Error" || c.State == "Failed" {
			return false, fmt.Errorf("cluster entered state %q", c.State)
		}
		return c.State == "Running" && clusterVersionMatches(c, target), nil
	})
	if err != nil {
		return nil, err
	}
	return latest, nil
}

// waitForPostActionRunning waits for a lifecycle action that does not expose a
// directly comparable target field in the cluster response. The first Running
// read is treated as pre-action state and one extra poll is required, giving the
// control plane time to move through its transient state before the next action.
func (r *kubernetesClusterResource) waitForPostActionRunning(ctx context.Context, slug string) (*kubernetes.Cluster, error) {
	var latest *kubernetes.Cluster
	seenFirstRunning := false
	err := pollUntilReady(ctx, kubernetesPollInterval, func(ctx context.Context) (bool, error) {
		c, err := r.svc.Get(ctx, slug)
		if err != nil {
			return false, err
		}
		latest = c
		if c.State == "Error" || c.State == "Failed" {
			return false, fmt.Errorf("cluster entered state %q", c.State)
		}
		if c.State != "Running" {
			seenFirstRunning = true
			return false, nil
		}
		if !seenFirstRunning {
			seenFirstRunning = true
			return false, nil
		}
		return true, nil
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
	if w := clusterWorkers(c); w > 0 && !model.Autoscaling.ValueBool() {
		model.Workers = types.Int64Value(w)
	}
	if c.ControlNodes > 0 {
		model.ControlNodes = types.Int64Value(int64(c.ControlNodes))
	} else if c.Meta != nil {
		if count, err := strconv.ParseInt(c.Meta.ControlNodes, 10, 64); err == nil && count > 0 {
			model.ControlNodes = types.Int64Value(count)
		}
	}
	if c.Offering != nil {
		if (model.ControlPlanePlan.IsNull() || model.ControlPlanePlan.IsUnknown()) && c.Offering.MasterPlan != nil {
			model.ControlPlanePlan = types.StringValue(kubernetesFirstNonEmpty(c.Offering.MasterPlan.Slug, c.Offering.MasterPlan.Name))
		}
		if (model.WorkerPlan.IsNull() || model.WorkerPlan.IsUnknown()) && c.Offering.WorkerPlan != nil {
			model.WorkerPlan = types.StringValue(kubernetesFirstNonEmpty(c.Offering.WorkerPlan.Slug, c.Offering.WorkerPlan.Name))
		}
	}
	// ha is create-only (RequiresReplace). The API may report it differently from
	// the submitted value (e.g. HA implicitly enabled for control_nodes >= 3),
	// which would otherwise produce a perpetual forced-replace diff, so preserve
	// the configured value and only adopt the API value when state has none (import).
	if (model.HA.IsNull() || model.HA.IsUnknown()) && c.EnableHAKnown {
		model.HA = types.BoolValue(c.EnableHA)
	}
	// version, plan, billing_cycle, storage_category, cloud_provider, project and
	// ssh_key are create-only; preserved from state to avoid format-driven diffs.
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func kubernetesFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// Update applies mutable lifecycle changes one at a time. The API rejects
// concurrent lifecycle actions, so each asynchronous request must settle before
// the next request is sent.
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
	if err := validateKubernetesAutoscaling(&model); err != nil {
		resp.Diagnostics.AddError("Invalid Kubernetes autoscaling configuration", err.Error())
		return
	}
	if state.Autoscaling.ValueBool() && model.Autoscaling.ValueBool() && model.Workers.ValueInt64() != state.Workers.ValueInt64() {
		resp.Diagnostics.AddError("Workers cannot change while autoscaling is enabled", "Disable autoscaling before changing workers.")
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

	// ready carries the freshest cluster read from whichever lifecycle action
	// ran last, so computed fields are written from live data, not stale state.
	var ready *kubernetes.Cluster
	autoscalingDisabled := false
	if model.Autoscaling.ValueBool() != state.Autoscaling.ValueBool() ||
		(model.Autoscaling.ValueBool() && (model.MinWorkers.ValueInt64() != state.MinWorkers.ValueInt64() || model.MaxWorkers.ValueInt64() != state.MaxWorkers.ValueInt64())) {
		if model.Autoscaling.ValueBool() {
			if err := r.svc.EnableAutoscaling(ctx, slug, int(model.MinWorkers.ValueInt64()), int(model.MaxWorkers.ValueInt64())); err != nil {
				resp.Diagnostics.AddError("Failed to enable Kubernetes autoscaling", err.Error())
				return
			}
			configured, err := r.waitForAutoscaling(ctx, slug, true, model.MinWorkers.ValueInt64(), model.MaxWorkers.ValueInt64())
			if err != nil {
				resp.Diagnostics.AddError("Kubernetes autoscaling configuration did not complete", err.Error())
				return
			}
			ready = configured
		} else {
			current, err := r.svc.Get(ctx, slug)
			if err != nil {
				resp.Diagnostics.AddError("Failed to read Kubernetes cluster before disabling autoscaling", err.Error())
				return
			}
			actualWorkers := clusterWorkers(current)
			if actualWorkers < 1 {
				resp.Diagnostics.AddError("Cannot disable Kubernetes autoscaling", "The API did not report a positive current worker count.")
				return
			}
			if err := r.svc.DisableAutoscaling(ctx, slug, int(actualWorkers)); err != nil {
				resp.Diagnostics.AddError("Failed to disable Kubernetes autoscaling", err.Error())
				return
			}
			_, err = r.waitForAutoscaling(ctx, slug, false, 0, 0)
			if err != nil {
				resp.Diagnostics.AddError("Kubernetes autoscaling disable did not complete", err.Error())
				return
			}
			ready, err = r.waitForScale(ctx, slug, actualWorkers)
			if err != nil {
				resp.Diagnostics.AddError("Kubernetes autoscaling disable did not complete", err.Error())
				return
			}
			if int64(desired) != actualWorkers {
				if err := r.svc.Scale(ctx, slug, desired); err != nil {
					resp.Diagnostics.AddError("Failed to scale Kubernetes cluster after disabling autoscaling", err.Error())
					return
				}
				ready, err = r.waitForScale(ctx, slug, int64(desired))
				if err != nil {
					resp.Diagnostics.AddError("Kubernetes worker resize after autoscaling disable did not complete", err.Error())
					return
				}
			}
			autoscalingDisabled = true
		}
	}

	// Version upgrade first: upgrading nodes before resizing them keeps both
	// operations independent, and the API rejects concurrent lifecycle actions.
	if model.Version.ValueString() != state.Version.ValueString() {
		current, err := r.svc.Get(ctx, slug)
		if err != nil {
			resp.Diagnostics.AddError("Failed to read Kubernetes cluster before version upgrade", err.Error())
			return
		}
		target, err := r.resolveVersionForCluster(ctx, current, model.Version.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Failed to resolve Kubernetes version", err.Error())
			return
		}
		if err := r.svc.UpgradeVersion(ctx, slug, kubernetes.UpgradeVersionRequest{
			Slug: target.Slug,
		}); err != nil {
			resp.Diagnostics.AddError("Failed to upgrade Kubernetes version", err.Error())
			return
		}
		upgraded, err := r.waitForVersion(ctx, slug, target)
		if err != nil {
			resp.Diagnostics.AddError("Kubernetes version upgrade did not complete", err.Error())
			return
		}
		ready = upgraded
	}

	if !model.Plan.IsNull() && !model.Plan.IsUnknown() && model.Plan.ValueString() != state.Plan.ValueString() {
		if err := r.svc.Upgrade(ctx, slug, kubernetes.UpgradeRequest{
			Plan:         model.Plan.ValueString(),
			Slug:         slug,
			BillingCycle: model.BillingCycle.ValueString(),
			IsCustomPlan: false,
			CustomPlan:   nil,
		}); err != nil {
			resp.Diagnostics.AddError("Failed to change Kubernetes cluster plan", err.Error())
			return
		}
		var err error
		ready, err = r.waitForPostActionRunning(ctx, slug)
		if err != nil {
			resp.Diagnostics.AddError("Kubernetes plan upgrade did not complete", err.Error())
			return
		}
	}

	if !model.Autoscaling.ValueBool() && !autoscalingDisabled && desired != int(state.Workers.ValueInt64()) {
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
		normalizeKubernetesUpdateState(&model, &state)
		resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
		return
	}

	if ready != nil {
		r.applyClusterState(&model, ready)
	} else {
		// No lifecycle action changed the computed fields.
		model.ID = state.ID
		model.Slug = state.Slug
		model.State = state.State
		model.APIEndpoint = state.APIEndpoint
		model.IPAddress = state.IPAddress
		model.NetworkName = state.NetworkName
		model.NetworkSlug = state.NetworkSlug
		model.RootVolumeSlugs = state.RootVolumeSlugs
	}
	model.ControlNodes = state.ControlNodes
	model.HA = state.HA
	normalizeKubernetesUpdateState(&model, &state)
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
	cluster, err := r.svc.Get(deleteCtx, slug)
	if err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to read Kubernetes cluster before deletion", err.Error())
		return
	}
	if err := validateKubernetesDeleteState(cluster); err != nil {
		resp.Diagnostics.AddError("Kubernetes cluster cannot be deleted", err.Error())
		return
	}
	if cluster.Slug == "" {
		cluster.Slug = slug
	}
	if strings.EqualFold(cluster.State, "Destroying") || (cluster.Meta != nil && strings.EqualFold(cluster.Meta.State, "Destroying")) {
		// A cancellation is already in progress. Poll it instead of submitting a
		// duplicate cancellation request.
	} else {
		err = r.svc.Cancel(deleteCtx, cluster)
	}
	if err != nil && !apierrors.IsNotFound(err) && !apierrors.IsResourceNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete Kubernetes cluster", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, kubernetesPollInterval, func(ctx context.Context) (bool, error) {
		_, err := r.svc.Get(ctx, slug)
		if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
			return false, nil
		}
		return err == nil, err
	}); err != nil {
		resp.Diagnostics.AddError("Cluster deletion did not complete", err.Error())
	}
}

func validateKubernetesDeleteState(cluster *kubernetes.Cluster) error {
	state := ""
	if cluster != nil && cluster.Meta != nil {
		state = strings.TrimSpace(cluster.Meta.State)
	}
	if state == "" && cluster != nil {
		state = strings.TrimSpace(cluster.State)
	}
	if state == "" {
		return fmt.Errorf("the current state is unknown; refresh the cluster and retry when its state is available")
	}
	switch strings.ToLower(state) {
	case "starting":
		return fmt.Errorf("cluster is Starting; wait for provisioning to finish before deleting")
	}
	return nil
}

// ImportState seeds the create-only attributes the API does not return in a
// comparable form so a post-import plan is zero-diff. Format (slash-separated,
// trailing/empty segments allowed):
//
//	<slug>/<cloud_provider>/<region>/<version>/<plan>/<billing_cycle>/<storage_category>/<project>/<ssh_key>
//
// Legacy imports use a single plan. Split-plan imports optionally append an
// explicit HA value when the API omits enable_ha.
func (r *kubernetesClusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) == 13 || len(parts) == 14 {
		fields := []string{"id", "cloud_provider", "region", "version", "control_plane_plan", "worker_plan", "storage_plan", "root_disk_size", "billing_cycle", "storage_category", "project", "ssh_key", "enable_csi"}
		if len(parts) == 14 {
			fields = append(fields, "ha")
		}
		for i, field := range fields {
			if strings.TrimSpace(parts[i]) == "" && field != "project" && field != "ssh_key" {
				resp.Diagnostics.AddError("Invalid import ID", "all v1 Kubernetes import fields are required")
				return
			}
			if strings.TrimSpace(parts[i]) == "" {
				continue
			}
			if field == "root_disk_size" {
				n, err := strconv.ParseInt(parts[i], 10, 64)
				if err != nil || n < 1 {
					resp.Diagnostics.AddError("Invalid import ID", "root_disk_size must be a positive integer")
					return
				}
				resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(field), n)...)
				continue
			}
			if field == "enable_csi" || field == "ha" {
				b, err := strconv.ParseBool(parts[i])
				if err != nil {
					resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("%s must be true or false", field))
					return
				}
				resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(field), b)...)
				continue
			}
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(field), strings.TrimSpace(parts[i]))...)
		}
		return
	}
	if len(parts) > 9 {
		resp.Diagnostics.AddError("Invalid import ID", "expected either the legacy import format or 13/14 split-plan fields")
		return
	}
	fields := []string{"id", "cloud_provider", "region", "version", "plan", "billing_cycle", "storage_category", "project", "ssh_key"}
	importPositional(ctx, req, resp, fields, 7,
		"<slug>/<cloud_provider>/<region>/<version>/<plan>/<billing_cycle>/<storage_category>[/<project>/<ssh_key>]")
}
