package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/loadbalancer"
)

var _ resource.Resource = &loadBalancerResource{}
var _ resource.ResourceWithConfigure = &loadBalancerResource{}
var _ resource.ResourceWithImportState = &loadBalancerResource{}
var _ resource.ResourceWithValidateConfig = &loadBalancerResource{}

// loadBalancerServiceIface is shared by zcp_load_balancer, zcp_load_balancer_rule,
// and zcp_load_balancer_attachment.
type loadBalancerServiceIface interface {
	List(ctx context.Context, region, project string) ([]loadbalancer.LoadBalancer, error)
	Create(ctx context.Context, req loadbalancer.CreateRequest) (*loadbalancer.LoadBalancer, error)
	Delete(ctx context.Context, slug string) error
	CreateRule(ctx context.Context, lbSlug string, req loadbalancer.CreateRuleRequest) error
	DeleteRule(ctx context.Context, lbSlug, ruleID string) error
	AttachVM(ctx context.Context, lbSlug, ruleID string, req loadbalancer.AttachVMRequest) error
	DetachVM(ctx context.Context, lbSlug, ruleID, vmSlug string) error
}

type loadBalancerResource struct {
	svc            loadBalancerServiceIface
	defaultProject string
}

type loadBalancerResourceModel struct {
	ID              types.String   `tfsdk:"id"`
	Name            types.String   `tfsdk:"name"`
	CloudProvider   types.String   `tfsdk:"cloud_provider"`
	Region          types.String   `tfsdk:"region"`
	Project         types.String   `tfsdk:"project"`
	Network         types.String   `tfsdk:"network"`
	Plan            types.String   `tfsdk:"plan"`
	BillingCycle    types.String   `tfsdk:"billing_cycle"`
	AcquireNewIP    types.Bool     `tfsdk:"acquire_new_ip"`
	IPAddress       types.String   `tfsdk:"ip_address"`
	RuleName        types.String   `tfsdk:"rule_name"`
	PublicPort      types.String   `tfsdk:"public_port"`
	PrivatePort     types.String   `tfsdk:"private_port"`
	Protocol        types.String   `tfsdk:"protocol"`
	Algorithm       types.String   `tfsdk:"algorithm"`
	StickyMethod    types.String   `tfsdk:"sticky_method"`
	EnableTLS       types.Bool     `tfsdk:"enable_tls"`
	EnableProxy     types.Bool     `tfsdk:"enable_proxy"`
	VirtualMachines types.List     `tfsdk:"virtual_machines"`
	State           types.String   `tfsdk:"state"`
	PublicIP        types.String   `tfsdk:"public_ip"`
	RuleID          types.String   `tfsdk:"rule_id"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
}

func NewLoadBalancerResource() resource.Resource {
	return &loadBalancerResource{}
}

func (r *loadBalancerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_load_balancer"
}

func (r *loadBalancerResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP load balancer with its initial rule. Add further rules with " +
			"`zcp_load_balancer_rule` and attach instances with `zcp_load_balancer_attachment`. " +
			"The API has no update endpoint for load balancers, so every change forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Load balancer slug.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name for the load balancer. Changing this forces replacement.",
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
			"network": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Slug of the network the balanced instances live in. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"plan": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Load balancer plan slug. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"billing_cycle": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Billing cycle (e.g. `hourly`, `monthly`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"acquire_new_ip": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Acquire a new public IP for the load balancer. Defaults to `true` when `ip_address` is not set; conflicts with `ip_address`. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"ip_address": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Existing public IP slug to bind instead of acquiring a new one. Conflicts with `acquire_new_ip = true`. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"rule_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the initial load balancing rule. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"public_port": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Public port of the initial rule (e.g. `443`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"private_port": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Private port of the initial rule (e.g. `8443`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"protocol": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Protocol of the initial rule (e.g. `tcp`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"algorithm": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Balancing algorithm: `roundrobin`, `leastconn`, or `source`. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"sticky_method": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Session stickiness method (e.g. `LbCookie`, `SourceBased`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enable_tls": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable TLS on the initial rule. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"enable_proxy": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable the PROXY protocol on the initial rule. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"virtual_machines": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Instance slugs to attach to the initial rule at create time. Manage attachments after create with `zcp_load_balancer_attachment`. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.List{listplanmodifier.RequiresReplace()},
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current state of the load balancer.",
			},
			"public_ip": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Public IP address bound to the load balancer.",
			},
			"rule_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the initial rule, usable with `zcp_load_balancer_attachment`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
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

func (r *loadBalancerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = loadbalancer.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

// ValidateConfig rejects binding an existing IP while also acquiring a new one.
// When neither is set, Create defaults to acquiring a new IP.
func (r *loadBalancerResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var model loadBalancerResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	acquire := !model.AcquireNewIP.IsNull() && !model.AcquireNewIP.IsUnknown() && model.AcquireNewIP.ValueBool()
	ipSet := !model.IPAddress.IsNull() && !model.IPAddress.IsUnknown()
	if acquire && ipSet {
		resp.Diagnostics.AddError(
			"Conflicting IP configuration",
			"`acquire_new_ip = true` (allocate a fresh public IP) and `ip_address` (bind an existing public IP) are mutually exclusive — set only one.",
		)
	}
}

func (r *loadBalancerResource) projectOrDefault(project types.String) string {
	if !project.IsNull() && !project.IsUnknown() {
		return project.ValueString()
	}
	return r.defaultProject
}

// applyLBState populates computed attributes from a LoadBalancer.
func (r *loadBalancerResource) applyLBState(model *loadBalancerResourceModel, lb *loadbalancer.LoadBalancer) {
	model.ID = types.StringValue(lb.Slug)
	model.State = types.StringValue(lb.State)
	if lb.IPAddress != nil && lb.IPAddress.IPAddress != "" {
		model.PublicIP = types.StringValue(lb.IPAddress.IPAddress)
	} else {
		model.PublicIP = types.StringNull()
	}
	// Resolve the initial rule's ID by name so attachments can reference it.
	ruleName := model.RuleName.ValueString()
	ruleID := types.StringNull()
	for _, rule := range lb.Rules {
		if rule.Name == ruleName {
			ruleID = types.StringValue(rule.ID)
			break
		}
	}
	if !ruleID.IsNull() || model.RuleID.IsUnknown() {
		model.RuleID = ruleID
	}
}

func (r *loadBalancerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model loadBalancerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_load_balancer cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	ruleSpec := loadbalancer.CreateRuleSpec{
		Name:            model.RuleName.ValueString(),
		PublicPort:      model.PublicPort.ValueString(),
		PrivatePort:     model.PrivatePort.ValueString(),
		Protocol:        model.Protocol.ValueString(),
		Algorithm:       model.Algorithm.ValueString(),
		StickyMethod:    model.StickyMethod.ValueString(),
		VirtualMachines: []loadbalancer.VMAttachment{},
	}
	if !model.EnableTLS.IsNull() && !model.EnableTLS.IsUnknown() {
		ruleSpec.EnableTLSProtocol = model.EnableTLS.ValueBool()
	}
	if !model.EnableProxy.IsNull() && !model.EnableProxy.IsUnknown() {
		ruleSpec.EnableProxyProtocol = model.EnableProxy.ValueBool()
	}
	if !model.VirtualMachines.IsNull() && !model.VirtualMachines.IsUnknown() {
		var vms []string
		resp.Diagnostics.Append(model.VirtualMachines.ElementsAs(ctx, &vms, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for _, vm := range vms {
			ruleSpec.VirtualMachines = append(ruleSpec.VirtualMachines, loadbalancer.VMAttachment{Slug: vm})
		}
	}

	createReq := loadbalancer.CreateRequest{
		Name:          model.Name.ValueString(),
		CloudProvider: model.CloudProvider.ValueString(),
		Project:       r.projectOrDefault(model.Project),
		Region:        model.Region.ValueString(),
		Network:       model.Network.ValueString(),
		Plan:          model.Plan.ValueString(),
		BillingCycle:  model.BillingCycle.ValueString(),
		Rules:         []loadbalancer.CreateRuleSpec{ruleSpec},
	}
	if !model.IPAddress.IsNull() && !model.IPAddress.IsUnknown() {
		ip := model.IPAddress.ValueString()
		createReq.IPAddress = &ip
	} else {
		// acquire_new_ip defaults to true when no existing IP is bound; an
		// explicit false with no ip_address is passed through and rejected by
		// the API, which requires one or the other.
		createReq.AcquireNewIP = model.AcquireNewIP.IsNull() || model.AcquireNewIP.IsUnknown() || model.AcquireNewIP.ValueBool()
	}

	lb, err := r.svc.Create(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create load balancer", err.Error())
		return
	}

	// The create response can omit nested relations (rules, IP). Refresh from the
	// list so rule_id and public_ip are populated in the initial state.
	if len(lb.Rules) == 0 || lb.IPAddress == nil {
		if full := r.findLB(ctx, lb.Slug, model.Region.ValueString(), r.projectOrDefault(model.Project)); full != nil {
			lb = full
		}
	}

	r.applyLBState(&model, lb)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// findLB returns the load balancer with the given slug, or nil when unavailable.
func (r *loadBalancerResource) findLB(ctx context.Context, slug, region, project string) *loadbalancer.LoadBalancer {
	lbs, err := r.svc.List(ctx, region, project)
	if err != nil {
		return nil
	}
	for i := range lbs {
		if lbs[i].Slug == slug {
			return &lbs[i]
		}
	}
	return nil
}

func (r *loadBalancerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model loadBalancerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_load_balancer cannot be read: bearer_token is missing.")
		return
	}

	lbs, err := r.svc.List(ctx, model.Region.ValueString(), r.projectOrDefault(model.Project))
	if err != nil {
		resp.Diagnostics.AddError("Failed to read load balancer", err.Error())
		return
	}

	slug := model.ID.ValueString()
	for i := range lbs {
		if lbs[i].Slug == slug {
			lb := &lbs[i]
			model.Name = types.StringValue(lb.Name)
			r.applyLBState(&model, lb)
			// cloud_provider, network, plan, billing_cycle, ip config, and the
			// initial rule inputs are write-only; preserved from state.
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *loadBalancerResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are ForceNew; Terraform never invokes this method.
}

func (r *loadBalancerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model loadBalancerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_load_balancer cannot be deleted: bearer_token is missing.")
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
		resp.Diagnostics.AddError("Failed to delete load balancer", err.Error())
		return
	}

	region := model.Region.ValueString()
	project := r.projectOrDefault(model.Project)
	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		lbs, err := r.svc.List(ctx, region, project)
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for i := range lbs {
			if lbs[i].Slug == slug {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError("Load balancer deletion did not complete", err.Error())
	}
}

// ImportState accepts a composite ID so the write-only create attributes are
// seeded for a zero-diff plan after import. Format:
//
//	<slug>/<cloud_provider>/<region>/<network>/<billing_cycle>/<rule_name>/<public_port>/<private_port>/<algorithm>[/<plan>/<project>]
//
// name, state, public_ip, and rule_id come from the subsequent Read.
func (r *loadBalancerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"id", "cloud_provider", "region", "network", "billing_cycle", "rule_name", "public_port", "private_port", "algorithm", "plan", "project"}
	importPositional(ctx, req, resp, fields, 9,
		"<slug>/<cloud_provider>/<region>/<network>/<billing_cycle>/<rule_name>/<public_port>/<private_port>/<algorithm>[/<plan>/<project>]")
}
