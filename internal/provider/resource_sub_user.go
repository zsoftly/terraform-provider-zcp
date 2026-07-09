package provider

import (
	"context"
	"fmt"
	"time"
	"unicode"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/subuser"
)

var _ resource.Resource = &subUserResource{}
var _ resource.ResourceWithConfigure = &subUserResource{}
var _ resource.ResourceWithImportState = &subUserResource{}
var _ resource.ResourceWithValidateConfig = &subUserResource{}

type subUserServiceIface interface {
	List(ctx context.Context) ([]subuser.SubUser, error)
	Create(ctx context.Context, req subuser.CreateRequest) (*subuser.SubUser, error)
	Update(ctx context.Context, id string, req subuser.UpdateRequest) (*subuser.SubUser, error)
	Delete(ctx context.Context, id string) error
}

type subUserResource struct {
	svc subUserServiceIface
}

type subUserResourceModel struct {
	ID        types.String   `tfsdk:"id"`
	Name      types.String   `tfsdk:"name"`
	Email     types.String   `tfsdk:"email"`
	Password  types.String   `tfsdk:"password"`
	Role      types.String   `tfsdk:"role"`
	Projects  types.List     `tfsdk:"projects"`
	IsBlocked types.Bool     `tfsdk:"is_blocked"`
	Status    types.String   `tfsdk:"status"`
	Timeouts  timeouts.Value `tfsdk:"timeouts"`
}

func NewSubUserResource() resource.Resource {
	return &subUserResource{}
}

func (r *subUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sub_user"
}

func (r *subUserResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP sub-user (team member). `name`, `email`, `role`, `projects`, and " +
			"`is_blocked` update in place. The API cannot change a password after creation, so changing " +
			"`password` forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Sub-user ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name. Updated in place.",
			},
			"email": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Company email address. Updated in place.",
			},
			"password": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "Initial password (8+ characters with upper, lower, digit, and special character). Write-only. Changing this forces replacement.",
				// The API never returns the password, so an imported user has a
				// null prior value. Replacing only when a prior value exists
				// lets the first apply after import adopt the configured
				// password into state (the in-place update never sends it),
				// while real password changes still replace.
				PlanModifiers: []planmodifier.String{requiresReplaceUnlessAdopting()},
			},
			"role": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Role slug (e.g. from `zcp_role`). Updated in place.",
			},
			"projects": schema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Project slugs the sub-user has access to. Updated in place.",
			},
			"is_blocked": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether the sub-user is blocked from signing in. Updated in place.",
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current user status.",
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

// ValidateConfig enforces the documented password complexity before the API
// rejects it at apply time: 8+ characters with upper, lower, digit, and
// special character.
func (r *subUserResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var model subUserResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if model.Password.IsNull() || model.Password.IsUnknown() {
		return
	}
	pw := model.Password.ValueString()
	var upper, lower, digit, special bool
	for _, r := range pw {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		default:
			special = true
		}
	}
	if len(pw) < 8 || !upper || !lower || !digit || !special {
		resp.Diagnostics.AddAttributeError(
			path.Root("password"),
			"Password does not meet complexity requirements",
			"The password must be at least 8 characters and contain an uppercase letter, a lowercase letter, a digit, and a special character.",
		)
	}
}

func (r *subUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = subuser.NewService(pd.Client)
}

// applySubUserState refreshes the readable attributes from a SubUser.
func applySubUserState(ctx context.Context, model *subUserResourceModel, u *subuser.SubUser) diag.Diagnostics {
	model.ID = types.StringValue(u.ID)
	model.Name = types.StringValue(u.Name)
	model.Email = types.StringValue(u.Email)
	if slug := u.RoleSlug(); slug != "" {
		model.Role = types.StringValue(slug)
	}
	model.IsBlocked = preserveOptionalBool(model.IsBlocked, u.IsBlocked)
	if u.UserStatus != "" {
		model.Status = types.StringValue(u.UserStatus)
	} else {
		model.Status = types.StringNull()
	}
	projects, diags := types.ListValueFrom(ctx, types.StringType, u.ProjectSlugs())
	if !diags.HasError() {
		model.Projects = projects
	}
	return diags
}

// preserveOptionalBool keeps an optional bool null in state when the config
// left it unset and the API reports the zero value, avoiding a perpetual diff.
func preserveOptionalBool(current types.Bool, apiValue bool) types.Bool {
	if current.IsNull() && !apiValue {
		return types.BoolNull()
	}
	return types.BoolValue(apiValue)
}

func (r *subUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model subUserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_sub_user cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	var projects []string
	resp.Diagnostics.Append(model.Projects.ElementsAs(ctx, &projects, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := subuser.CreateRequest{
		Name:           model.Name.ValueString(),
		Email:          model.Email.ValueString(),
		Password:       model.Password.ValueString(),
		Role:           model.Role.ValueString(),
		Projects:       projects,
		AuthUser:       "customer",
		IsUserPassword: true,
	}
	if !model.IsBlocked.IsNull() && !model.IsBlocked.IsUnknown() {
		createReq.IsBlocked = model.IsBlocked.ValueBool()
	}

	created, err := r.svc.Create(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create sub-user", err.Error())
		return
	}

	if created.ID == "" {
		// Resolve from the list when the create response omits the ID.
		users, lerr := r.svc.List(ctx)
		if lerr != nil {
			resp.Diagnostics.AddError("Failed to resolve created sub-user", lerr.Error())
			return
		}
		for i := range users {
			if users[i].Email == model.Email.ValueString() {
				created = &users[i]
				break
			}
		}
	}
	if created.ID == "" {
		resp.Diagnostics.AddError(
			"Failed to resolve created sub-user",
			fmt.Sprintf("sub-user %q was created but does not appear in the user list.", model.Email.ValueString()),
		)
		return
	}

	model.ID = types.StringValue(created.ID)
	if created.UserStatus != "" {
		model.Status = types.StringValue(created.UserStatus)
	} else {
		model.Status = types.StringNull()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *subUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model subUserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_sub_user cannot be read: bearer_token is missing.")
		return
	}

	users, err := r.svc.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read sub-user", err.Error())
		return
	}

	id := model.ID.ValueString()
	for i := range users {
		if users[i].ID == id {
			resp.Diagnostics.Append(applySubUserState(ctx, &model, &users[i])...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *subUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state subUserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_sub_user cannot be updated: bearer_token is missing.")
		return
	}

	var projects []string
	resp.Diagnostics.Append(plan.Projects.ElementsAs(ctx, &projects, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The API requires email and projects on every update, so both are always
	// sent at their desired values.
	updateReq := subuser.UpdateRequest{
		Name:     plan.Name.ValueString(),
		Email:    plan.Email.ValueString(),
		Role:     plan.Role.ValueString(),
		Projects: projects,
	}
	if !plan.IsBlocked.IsNull() && !plan.IsBlocked.IsUnknown() {
		updateReq.IsBlocked = plan.IsBlocked.ValueBool()
	}

	if _, err := r.svc.Update(ctx, state.ID.ValueString(), updateReq); err != nil {
		resp.Diagnostics.AddError("Failed to update sub-user", err.Error())
		return
	}

	model := plan
	model.ID = state.ID
	model.Status = state.Status
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *subUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model subUserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_sub_user cannot be deleted: bearer_token is missing.")
		return
	}

	deleteTimeout, diags := model.Timeouts.Delete(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	id := model.ID.ValueString()
	err := r.svc.Delete(deleteCtx, id)
	if err != nil && !isBackendNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete sub-user", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		users, err := r.svc.List(ctx)
		if isBackendNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for i := range users {
			if users[i].ID == id {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError("Sub-user deletion did not complete", err.Error())
	}
}

// ImportState uses the sub-user ID. password cannot be imported (the API never
// returns it); the first apply after import adopts the configured password
// into state without replacing the user, because its RequiresReplaceIf only
// fires when a prior value exists in state.
func (r *subUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
