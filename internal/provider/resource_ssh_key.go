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
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/sshkey"
)

var _ resource.Resource = &sshKeyResource{}
var _ resource.ResourceWithConfigure = &sshKeyResource{}
var _ resource.ResourceWithImportState = &sshKeyResource{}

type sshKeyServiceIface interface {
	List(ctx context.Context) ([]sshkey.SSHKey, error)
	Create(ctx context.Context, req sshkey.CreateRequest) (*sshkey.SSHKey, error)
	Delete(ctx context.Context, keyID string) error
}

type sshKeyResource struct {
	svc            sshKeyServiceIface
	defaultProject string
}

type sshKeyResourceModel struct {
	ID        types.String   `tfsdk:"id"`
	Name      types.String   `tfsdk:"name"`
	PublicKey types.String   `tfsdk:"public_key"`
	Region    types.String   `tfsdk:"region"`
	Project   types.String   `tfsdk:"project"`
	CreatedAt types.String   `tfsdk:"created_at"`
	Timeouts  timeouts.Value `tfsdk:"timeouts"`
}

func NewSSHKeyResource() resource.Resource {
	return &sshKeyResource{}
}

func (r *sshKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssh_key"
}

func (r *sshKeyResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP SSH key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SSH key slug.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name for the SSH key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"public_key": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "OpenSSH public key material.",
				// The API returns only a normalized form of the key, so the
				// config value is never refreshed from it and an imported key
				// has a null prior value. Replacing only when a prior value
				// exists lets the first apply after import adopt the configured
				// key into state, while real key changes still replace.
				PlanModifiers: []planmodifier.String{requiresReplaceUnlessAdopting()},
			},
			"region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Region slug (e.g. `yow-1`). Required by the API to derive the cloud provider; changing it forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project slug. Inherits from the provider `default_project` if omitted. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Creation timestamp.",
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

func (r *sshKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = sshkey.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

func (r *sshKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model sshKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_ssh_key cannot be created: bearer_token is missing.")
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

	key, err := r.svc.Create(ctx, sshkey.CreateRequest{
		Name:      model.Name.ValueString(),
		PublicKey: model.PublicKey.ValueString(),
		Region:    model.Region.ValueString(),
		Project:   project,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create SSH key", err.Error())
		return
	}

	model.ID = types.StringValue(key.Slug)
	model.CreatedAt = types.StringValue(key.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *sshKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model sshKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_ssh_key cannot be read: bearer_token is missing.")
		return
	}

	keys, err := r.svc.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read SSH key", err.Error())
		return
	}

	slug := model.ID.ValueString()
	for _, k := range keys {
		if k.Slug == slug {
			model.Name = types.StringValue(k.Name)
			// public_key is write-only: the API returns a normalized form (no
			// trailing newline/comment) that never string-matches the config
			// (e.g. file("key.pub")), which would otherwise plan a replacement
			// on every refresh. Preserve the state value instead.
			model.CreatedAt = types.StringValue(k.CreatedAt)
			// project is write-only (not in API response); preserved from state.
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

// Update only runs for the adoption plan right after an import: public_key is
// write-only, and its plan modifier downgrades the diff from replacement to
// in-place when the prior value is null. Copying the plan into state records
// the configured key; nothing is sent to the API. Every other attribute change
// forces replacement, so Terraform never routes it here.
func (r *sshKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model sshKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *sshKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model sshKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_ssh_key cannot be deleted: bearer_token is missing.")
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
		resp.Diagnostics.AddError("Failed to delete SSH key", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		keys, err := r.svc.List(ctx)
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for _, k := range keys {
			if k.Slug == slug {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError("SSH key deletion did not complete", err.Error())
	}
}

// ImportState accepts a composite ID so the write-only region/project (which the
// API does not return) are seeded for a zero-diff plan after import. Format:
//
//	<slug>/<region>[/<project>]
//
// Omit <project> (or leave it empty: "<slug>/<region>/") when the config relies
// on the provider's default_project. name and created_at come from the
// subsequent Read. public_key cannot be imported (the API returns only a
// normalized form); the first apply after import adopts the configured key
// into state without replacing it.
func (r *sshKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"id", "region", "project"}
	importPositional(ctx, req, resp, fields, 2, "<slug>/<region>[/<project>]")
}
