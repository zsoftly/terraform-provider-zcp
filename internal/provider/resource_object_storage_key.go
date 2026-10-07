package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/objectstorage"
)

var _ resource.Resource = &objectStorageKeyResource{}
var _ resource.ResourceWithConfigure = &objectStorageKeyResource{}
var _ resource.ResourceWithImportState = &objectStorageKeyResource{}

type objectStorageKeyResource struct {
	svc objectStorageServiceIface
}

type objectStorageKeyResourceModel struct {
	ID                 types.String   `tfsdk:"id"`
	ObjectStorage      types.String   `tfsdk:"object_storage"`
	APIKey             types.String   `tfsdk:"api_key"`
	APISecret          types.String   `tfsdk:"api_secret"`
	Status             types.String   `tfsdk:"status"`
	IsPrimary          types.Bool     `tfsdk:"is_primary"`
	SecretVisibleUntil types.String   `tfsdk:"secret_visible_until"`
	CreatedAt          types.String   `tfsdk:"created_at"`
	UpdatedAt          types.String   `tfsdk:"updated_at"`
	Timeouts           timeouts.Value `tfsdk:"timeouts"`
}

func NewObjectStorageKeyResource() resource.Resource {
	return &objectStorageKeyResource{}
}

func (r *objectStorageKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_object_storage_key"
}

func (r *objectStorageKeyResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an S3 access key for a `zcp_object_storage` store. The secret is returned by the API only while its visibility window is open; after that, refresh preserves the secret already stored in Terraform state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Object storage key ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"object_storage": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Parent object storage slug. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"api_key": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "S3 access key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"api_secret": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "S3 secret key. It is stored in Terraform state after create because the API stops returning it when `secret_visible_until` expires.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Key status.",
			},
			"is_primary": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether this is the store's primary key.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"secret_visible_until": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC3339 timestamp until which the API may return the plaintext secret.",
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Creation timestamp.",
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Last update timestamp.",
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

func (r *objectStorageKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = objectstorage.NewService(pd.Client)
}

func (r *objectStorageKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model objectStorageKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_object_storage_key cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 2*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	key, err := r.svc.CreateKey(ctx, model.ObjectStorage.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create object storage key", err.Error())
		return
	}
	if key == nil || key.ID == "" {
		resp.Diagnostics.AddError("Failed to create object storage key", "the API accepted the create but returned no key ID; check the object storage key list before retrying.")
		return
	}
	if key.APISecret == "" || objectstorage.IsEncryptedSecret(key.APISecret) {
		creds, cerr := r.svc.GetCredentialsForKey(ctx, model.ObjectStorage.ValueString(), key.ID)
		if cerr == nil && creds != nil {
			key.APIKey = firstNonEmpty(key.APIKey, creds.APIKey)
			key.APISecret = creds.APISecret
			key.SecretVisibleUntil = firstNonEmpty(key.SecretVisibleUntil, creds.SecretVisibleUntil)
		}
	}
	if key.APISecret == "" || objectstorage.IsEncryptedSecret(key.APISecret) {
		r.cleanupCreatedKeyAfterSecretFailure(model.ObjectStorage.ValueString(), key.ID, resp)
		resp.Diagnostics.AddError("Object storage key secret not visible", "the key was created, but the API did not return a plaintext secret. Terraform attempted to revoke the new key before returning this error. If cleanup failed, revoke the key in ZCP before retrying so Terraform can store the secret in state.")
		return
	}

	applyObjectStorageKeyState(&model, key, false)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *objectStorageKeyResource) cleanupCreatedKeyAfterSecretFailure(objectStorage, keyID string, resp *resource.CreateResponse) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := r.svc.DeleteKey(cleanupCtx, objectStorage, keyID); err != nil {
		resp.Diagnostics.AddWarning(
			"Object storage key cleanup failed",
			fmt.Sprintf("The key %q was created, but Terraform could not capture a plaintext secret and failed to revoke the key automatically: %s. Revoke it in ZCP before retrying.", keyID, err),
		)
	}
}

func (r *objectStorageKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model objectStorageKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_object_storage_key cannot be read: bearer_token is missing.")
		return
	}

	keys, err := r.svc.ListKeys(ctx, model.ObjectStorage.ValueString())
	if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read object storage key", err.Error())
		return
	}
	for i := range keys {
		if keys[i].ID == model.ID.ValueString() {
			applyObjectStorageKeyState(&model, &keys[i], true)
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *objectStorageKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model objectStorageKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *objectStorageKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model objectStorageKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_object_storage_key cannot be deleted: bearer_token is missing.")
		return
	}

	deleteTimeout, diags := model.Timeouts.Delete(ctx, 2*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	err := r.svc.DeleteKey(ctx, model.ObjectStorage.ValueString(), model.ID.ValueString())
	if err != nil && !apierrors.IsNotFound(err) && !apierrors.IsResourceNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete object storage key", err.Error())
	}
}

func (r *objectStorageKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"object_storage", "id"}
	importPositional(ctx, req, resp, fields, 2, "<object_storage>/<key_id>")
}

func applyObjectStorageKeyState(model *objectStorageKeyResourceModel, key *objectstorage.Key, preserveSecret bool) {
	model.ID = types.StringValue(key.ID)
	if key.APIKey != "" {
		model.APIKey = types.StringValue(key.APIKey)
	}
	if key.APISecret != "" && !objectstorage.IsEncryptedSecret(key.APISecret) {
		model.APISecret = types.StringValue(key.APISecret)
	} else if !preserveSecret || model.APISecret.IsUnknown() {
		model.APISecret = types.StringNull()
	}
	if key.Status != "" {
		model.Status = types.StringValue(key.Status)
	} else {
		model.Status = types.StringNull()
	}
	model.IsPrimary = types.BoolValue(key.IsPrimary)
	model.SecretVisibleUntil = nullableString(key.SecretVisibleUntil)
	model.CreatedAt = nullableString(key.CreatedAt)
	model.UpdatedAt = nullableString(key.UpdatedAt)
}

func nullableString(v string) types.String {
	if strings.TrimSpace(v) == "" {
		return types.StringNull()
	}
	return types.StringValue(v)
}
