package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/project"
)

var _ resource.Resource = &projectResource{}
var _ resource.ResourceWithConfigure = &projectResource{}
var _ resource.ResourceWithImportState = &projectResource{}

type projectServiceIface interface {
	List(ctx context.Context) ([]project.Project, error)
	Create(ctx context.Context, req project.CreateRequest) (*project.Project, error)
	Update(ctx context.Context, slug string, req project.UpdateRequest) (*project.Project, error)
	Delete(ctx context.Context, slug string) error
}

type projectResource struct {
	svc projectServiceIface
}

type projectResourceModel struct {
	ID          types.String   `tfsdk:"id"`
	Name        types.String   `tfsdk:"name"`
	Description types.String   `tfsdk:"description"`
	Purpose     types.String   `tfsdk:"purpose"`
	Icon        types.String   `tfsdk:"icon"`
	Timeouts    timeouts.Value `tfsdk:"timeouts"`
}

func NewProjectResource() resource.Resource {
	return &projectResource{}
}

func (r *projectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *projectResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP project. `name`, `description`, and `purpose` update in place. " +
			"Deleting a project requires it to be empty of services.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Project slug.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Project display name. Updated in place.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Human-readable description. Updated in place when set. Removing it from configuration preserves the remote value because the API cannot clear it on update.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"purpose": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Project purpose. Updated in place when set. Removing it from configuration preserves the remote value because the API cannot clear it on update.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"icon": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Project icon identifier (see `zcp project icon list`). Defaults to `cloud-13`. Set at create time only; changing it forces replacement.",
				Default:             stringdefault.StaticString("cloud-13"),
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
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

func (r *projectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = project.NewService(pd.Client)
}

func (r *projectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model projectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_project cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	// Status 1 marks the project active, matching the CLI's fixed value.
	purpose := model.Purpose.ValueString()
	if purpose == "" {
		purpose = "Development & Testing"
	}
	icon := model.Icon.ValueString()
	if icon == "" {
		icon = "cloud-13"
	}
	createReq := project.CreateRequest{
		Name:    model.Name.ValueString(),
		Purpose: purpose,
		Icon:    icon,
		Status:  1,
	}
	if !model.Description.IsNull() && !model.Description.IsUnknown() {
		createReq.Description = model.Description.ValueString()
	}

	created, err := r.svc.Create(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create project", err.Error())
		return
	}

	slug := created.Slug
	if slug == "" {
		// Resolve from the list when the create response omits the slug.
		projects, lerr := r.svc.List(ctx)
		if lerr != nil {
			resp.Diagnostics.AddError("Failed to resolve created project", lerr.Error())
			return
		}
		for _, p := range projects {
			if p.Name == model.Name.ValueString() {
				slug = p.Slug
				break
			}
		}
		if slug == "" {
			resp.Diagnostics.AddError(
				"Failed to resolve created project",
				fmt.Sprintf("project %q was created but does not appear in the project list.", model.Name.ValueString()),
			)
			return
		}
	}

	model.ID = types.StringValue(slug)
	model.Purpose = types.StringValue(purpose)
	model.Icon = types.StringValue(icon)
	// description is computed, so an omitted value arrives unknown and must be
	// resolved before the state is written.
	if model.Description.IsUnknown() {
		if created.Description != "" {
			model.Description = types.StringValue(created.Description)
		} else {
			model.Description = types.StringNull()
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *projectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model projectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_project cannot be read: bearer_token is missing.")
		return
	}

	projects, err := r.svc.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read project", err.Error())
		return
	}

	slug := model.ID.ValueString()
	for _, p := range projects {
		if p.Slug == slug {
			model.Name = types.StringValue(p.Name)
			if p.Description != "" {
				model.Description = types.StringValue(p.Description)
			} else {
				model.Description = types.StringNull()
			}
			if p.Purpose != "" {
				model.Purpose = types.StringValue(p.Purpose)
			} else {
				model.Purpose = types.StringNull()
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *projectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state projectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_project cannot be updated: bearer_token is missing.")
		return
	}

	updateReq := project.UpdateRequest{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		updateReq.Description = plan.Description.ValueString()
	}
	if !plan.Purpose.IsNull() && !plan.Purpose.IsUnknown() {
		updateReq.Purpose = plan.Purpose.ValueString()
	}

	if _, err := r.svc.Update(ctx, state.ID.ValueString(), updateReq); err != nil {
		resp.Diagnostics.AddError("Failed to update project", err.Error())
		return
	}

	model := plan
	model.ID = state.ID
	// The API update request omits empty description/purpose values, so a
	// Terraform null cannot clear an existing remote value. Preserve prior state
	// instead of recording a value the API did not apply.
	if plan.Description.IsNull() || plan.Description.IsUnknown() {
		model.Description = state.Description
	}
	if plan.Purpose.IsNull() || plan.Purpose.IsUnknown() {
		model.Purpose = state.Purpose
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *projectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model projectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_project cannot be deleted: bearer_token is missing.")
		return
	}

	deleteTimeout, diags := model.Timeouts.Delete(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	slug := model.ID.ValueString()
	err := r.svc.Delete(deleteCtx, slug)
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete project", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		projects, err := r.svc.List(ctx)
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for _, p := range projects {
			if p.Slug == slug {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError("Project deletion did not complete", err.Error())
	}
}

func (r *projectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
