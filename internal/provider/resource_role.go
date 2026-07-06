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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/role"
)

var _ resource.Resource = &roleResource{}
var _ resource.ResourceWithConfigure = &roleResource{}
var _ resource.ResourceWithImportState = &roleResource{}

type roleServiceIface interface {
	Get(ctx context.Context, slug string) (*role.Role, error)
	Create(ctx context.Context, req role.CreateRequest) (*role.Role, error)
	Update(ctx context.Context, slug string, req role.UpdateRequest) (*role.Role, error)
	Delete(ctx context.Context, slug string) error
}

type roleResource struct {
	svc roleServiceIface
}

type roleResourceModel struct {
	ID          types.String   `tfsdk:"id"`
	Name        types.String   `tfsdk:"name"`
	Description types.String   `tfsdk:"description"`
	Permissions types.Set      `tfsdk:"permissions"`
	Timeouts    timeouts.Value `tfsdk:"timeouts"`
}

func NewRoleResource() resource.Resource {
	return &roleResource{}
}

func (r *roleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (r *roleResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP role for sub-users. `permissions` is the full desired set; updates replace the role's existing permissions. Use the `zcp_permissions` data source to discover permission slugs.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Role slug.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Role display name. Updated in place.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Human-readable description. Updated in place; removing it clears the description.",
			},
			"permissions": schema.SetAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Permission slugs granted by the role. The set replaces the role's permissions on update.",
				Validators:          []validator.Set{setSizeAtLeastValidator{min: 1}},
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

func (r *roleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = role.NewService(pd.Client)
}

func isRoleNotFound(err error) bool {
	return isBackendNotFound(err)
}

func (r *roleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model roleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_role cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	var permissions []string
	resp.Diagnostics.Append(model.Permissions.ElementsAs(ctx, &permissions, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(permissions) == 0 {
		resp.Diagnostics.AddError("Missing role permissions", "At least one permission is required.")
		return
	}

	createReq := role.CreateRequest{
		Name:        model.Name.ValueString(),
		Permissions: permissions,
	}
	if !model.Description.IsNull() && !model.Description.IsUnknown() {
		createReq.Description = model.Description.ValueString()
	}

	created, err := r.svc.Create(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create role", err.Error())
		return
	}
	if created.Slug == "" {
		resp.Diagnostics.AddError("Failed to resolve created role", "the API accepted the role but returned no slug.")
		return
	}

	model.ID = types.StringValue(created.Slug)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *roleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model roleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_role cannot be read: bearer_token is missing.")
		return
	}

	got, err := r.svc.Get(ctx, model.ID.ValueString())
	if isRoleNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read role", err.Error())
		return
	}

	model.Name = types.StringValue(got.Name)
	if got.Description != "" {
		model.Description = types.StringValue(got.Description)
	} else {
		model.Description = types.StringNull()
	}
	slugs := make([]string, 0, len(got.Permissions))
	for _, p := range got.Permissions {
		slugs = append(slugs, p.Slug)
	}
	permissions, diags := types.SetValueFrom(ctx, types.StringType, slugs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	model.Permissions = permissions
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *roleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state roleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_role cannot be updated: bearer_token is missing.")
		return
	}

	var permissions []string
	resp.Diagnostics.Append(plan.Permissions.ElementsAs(ctx, &permissions, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(permissions) == 0 {
		resp.Diagnostics.AddError("Missing role permissions", "At least one permission is required.")
		return
	}

	// UpdateRequest is full desired state: permissions replace the existing set
	// and an empty description clears it (the field has no omitempty on purpose).
	updateReq := role.UpdateRequest{
		Name:        plan.Name.ValueString(),
		Permissions: permissions,
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		updateReq.Description = plan.Description.ValueString()
	}

	if _, err := r.svc.Update(ctx, state.ID.ValueString(), updateReq); err != nil {
		resp.Diagnostics.AddError("Failed to update role", err.Error())
		return
	}

	model := plan
	model.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *roleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model roleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_role cannot be deleted: bearer_token is missing.")
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
	if err != nil && !isRoleNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete role", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		_, err := r.svc.Get(ctx, slug)
		if isRoleNotFound(err) {
			return false, nil
		}
		return err == nil, err
	}); err != nil {
		resp.Diagnostics.AddError("Role deletion did not complete", err.Error())
	}
}

func (r *roleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
