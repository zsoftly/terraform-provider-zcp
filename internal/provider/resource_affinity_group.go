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
	"github.com/zsoftly/zcp-cli/pkg/api/affinitygroup"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
)

var _ resource.Resource = &affinityGroupResource{}
var _ resource.ResourceWithConfigure = &affinityGroupResource{}
var _ resource.ResourceWithImportState = &affinityGroupResource{}

type affinityGroupServiceIface interface {
	List(ctx context.Context, region, project string) ([]affinitygroup.AffinityGroup, error)
	Create(ctx context.Context, req affinitygroup.CreateRequest) (*affinitygroup.AffinityGroup, error)
	Delete(ctx context.Context, slug string) error
}

type affinityGroupResource struct {
	svc            affinityGroupServiceIface
	defaultProject string
}

type affinityGroupResourceModel struct {
	ID            types.String   `tfsdk:"id"`
	Name          types.String   `tfsdk:"name"`
	Type          types.String   `tfsdk:"type"`
	Description   types.String   `tfsdk:"description"`
	CloudProvider types.String   `tfsdk:"cloud_provider"`
	Region        types.String   `tfsdk:"region"`
	Project       types.String   `tfsdk:"project"`
	State         types.String   `tfsdk:"state"`
	Timeouts      timeouts.Value `tfsdk:"timeouts"`
}

func NewAffinityGroupResource() resource.Resource {
	return &affinityGroupResource{}
}

func (r *affinityGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_affinity_group"
}

func (r *affinityGroupResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP affinity group. Attach instances at create time to influence host placement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Affinity group slug.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name for the affinity group. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Affinity group type (e.g. `host anti-affinity`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Human-readable description. Changing this forces replacement.",
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
				MarkdownDescription: "Current state of the affinity group.",
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

func (r *affinityGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = affinitygroup.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

// projectOrDefault returns the model's project when set, else the provider default.
func (r *affinityGroupResource) projectOrDefault(project types.String) string {
	if !project.IsNull() && !project.IsUnknown() {
		return project.ValueString()
	}
	return r.defaultProject
}

func (r *affinityGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model affinityGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_affinity_group cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	createReq := affinitygroup.CreateRequest{
		Name:          model.Name.ValueString(),
		Type:          model.Type.ValueString(),
		Project:       r.projectOrDefault(model.Project),
		Region:        model.Region.ValueString(),
		CloudProvider: model.CloudProvider.ValueString(),
	}
	if !model.Description.IsNull() && !model.Description.IsUnknown() {
		createReq.Description = model.Description.ValueString()
	}

	group, err := r.svc.Create(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create affinity group", err.Error())
		return
	}

	slug := group.Slug
	if slug == "" {
		// Some create responses return a partial object; resolve the slug from
		// the list so state never stores an empty ID.
		groups, lerr := r.svc.List(ctx, model.Region.ValueString(), r.projectOrDefault(model.Project))
		if lerr != nil {
			resp.Diagnostics.AddError("Failed to resolve created affinity group", lerr.Error())
			return
		}
		for _, g := range groups {
			// Names are not unique across types, so match both to avoid
			// binding a same-named group of a different type.
			if g.Name == createReq.Name && g.Type == createReq.Type {
				slug = g.Slug
				break
			}
		}
		if slug == "" {
			resp.Diagnostics.AddError(
				"Failed to resolve created affinity group",
				fmt.Sprintf("affinity group %q was created but does not appear in the list.", model.Name.ValueString()),
			)
			return
		}
	}

	model.ID = types.StringValue(slug)
	if group.State != "" {
		model.State = types.StringValue(group.State)
	} else if group.Status != "" {
		model.State = types.StringValue(group.Status)
	} else {
		model.State = types.StringNull()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *affinityGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model affinityGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_affinity_group cannot be read: bearer_token is missing.")
		return
	}

	groups, err := r.svc.List(ctx, model.Region.ValueString(), r.projectOrDefault(model.Project))
	if err != nil {
		resp.Diagnostics.AddError("Failed to read affinity group", err.Error())
		return
	}

	slug := model.ID.ValueString()
	for _, g := range groups {
		if g.Slug == slug {
			model.Name = types.StringValue(g.Name)
			if g.Type != "" {
				model.Type = types.StringValue(g.Type)
			}
			if g.Description != "" {
				model.Description = types.StringValue(g.Description)
			}
			if g.State != "" {
				model.State = types.StringValue(g.State)
			} else if g.Status != "" {
				model.State = types.StringValue(g.Status)
			}
			// cloud_provider, region, and project are write-only; preserved from state.
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *affinityGroupResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are ForceNew; Terraform never invokes this method.
}

func (r *affinityGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model affinityGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_affinity_group cannot be deleted: bearer_token is missing.")
		return
	}

	deleteTimeout, diags := model.Timeouts.Delete(ctx, 2*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	slug := model.ID.ValueString()
	err := r.svc.Delete(deleteCtx, slug)
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete affinity group", err.Error())
		return
	}

	region := model.Region.ValueString()
	project := r.projectOrDefault(model.Project)
	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		groups, err := r.svc.List(ctx, region, project)
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for _, g := range groups {
			if g.Slug == slug {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError("Affinity group deletion did not complete", err.Error())
	}
}

// ImportState accepts a composite ID so the write-only region/cloud_provider/project
// are seeded for a zero-diff plan after import. Format:
//
//	<slug>/<region>/<cloud_provider>[/<project>]
//
// Omit <project> when the config relies on the provider's default_project.
// name, type, description, and state come from the subsequent Read.
func (r *affinityGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"id", "region", "cloud_provider", "project"}
	importPositional(ctx, req, resp, fields, 3, "<slug>/<region>/<cloud_provider>[/<project>]")
}
