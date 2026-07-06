package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/loadbalancer"
)

var _ resource.Resource = &loadBalancerAttachmentResource{}
var _ resource.ResourceWithConfigure = &loadBalancerAttachmentResource{}
var _ resource.ResourceWithImportState = &loadBalancerAttachmentResource{}

type loadBalancerAttachmentResource struct {
	svc            loadBalancerServiceIface
	defaultProject string
}

type loadBalancerAttachmentResourceModel struct {
	ID             types.String   `tfsdk:"id"`
	LoadBalancer   types.String   `tfsdk:"load_balancer"`
	Rule           types.String   `tfsdk:"rule"`
	VirtualMachine types.String   `tfsdk:"virtual_machine"`
	CloudProvider  types.String   `tfsdk:"cloud_provider"`
	Region         types.String   `tfsdk:"region"`
	Project        types.String   `tfsdk:"project"`
	Timeouts       timeouts.Value `tfsdk:"timeouts"`
}

func NewLoadBalancerAttachmentResource() resource.Resource {
	return &loadBalancerAttachmentResource{}
}

func (r *loadBalancerAttachmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_load_balancer_attachment"
}

func (r *loadBalancerAttachmentResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Attaches a single instance to a load balancer rule. Use one attachment per " +
			"instance/rule pair (with `for_each` for fleets). The API does not expose rule membership, so " +
			"drift in attachments made or removed outside Terraform is not detected; the resource is removed " +
			"from state when the rule or load balancer disappears.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Synthetic attachment ID (`<load_balancer>/<rule>/<virtual_machine>`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"load_balancer": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Parent load balancer slug. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"rule": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Rule ID to attach to (e.g. `zcp_load_balancer.web.rule_id`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"virtual_machine": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Instance slug to attach. Changing this forces replacement.",
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

func (r *loadBalancerAttachmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *loadBalancerAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model loadBalancerAttachmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_load_balancer_attachment cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 5*time.Minute)
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

	lbSlug := model.LoadBalancer.ValueString()
	ruleID := model.Rule.ValueString()
	vmSlug := model.VirtualMachine.ValueString()

	err := r.svc.AttachVM(ctx, lbSlug, ruleID, loadbalancer.AttachVMRequest{
		VirtualMachines: []string{vmSlug},
		CloudProvider:   model.CloudProvider.ValueString(),
		Region:          model.Region.ValueString(),
		Project:         project,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to attach instance to load balancer rule", err.Error())
		return
	}

	model.ID = types.StringValue(fmt.Sprintf("%s/%s/%s", lbSlug, ruleID, vmSlug))
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *loadBalancerAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model loadBalancerAttachmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_load_balancer_attachment cannot be read: bearer_token is missing.")
		return
	}

	// The API does not expose rule membership, so the attachment itself cannot
	// be verified. Drop the resource when its parent rule or LB is gone; keep
	// state as-is otherwise.
	lbs, err := r.svc.List(ctx, model.Region.ValueString(), "")
	if err != nil {
		resp.Diagnostics.AddError("Failed to read load balancer attachment", err.Error())
		return
	}
	lb := findLBBySlug(lbs, model.LoadBalancer.ValueString())
	if lb == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	ruleID := model.Rule.ValueString()
	for _, rule := range lb.Rules {
		if rule.ID == ruleID {
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *loadBalancerAttachmentResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are ForceNew; Terraform never invokes this method.
}

func (r *loadBalancerAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model loadBalancerAttachmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_load_balancer_attachment cannot be deleted: bearer_token is missing.")
		return
	}

	deleteTimeout, diags := model.Timeouts.Delete(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	err := r.svc.DetachVM(deleteCtx, model.LoadBalancer.ValueString(), model.Rule.ValueString(), model.VirtualMachine.ValueString())
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to detach instance from load balancer rule", err.Error())
	}
}

// ImportState accepts a composite ID seeding the attachment triple plus the
// write-only scope fields. Format:
//
//	<load_balancer>/<rule_id>/<virtual_machine>/<cloud_provider>/<region>[/<project>]
func (r *loadBalancerAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"load_balancer", "rule", "virtual_machine", "cloud_provider", "region", "project"}
	importPositional(ctx, req, resp, fields, 5,
		"<load_balancer>/<rule_id>/<virtual_machine>/<cloud_provider>/<region>[/<project>]")
	if resp.Diagnostics.HasError() {
		return
	}
	parts := strings.SplitN(req.ID, "/", len(fields))
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"),
		fmt.Sprintf("%s/%s/%s", parts[0], parts[1], parts[2]))...)
}
