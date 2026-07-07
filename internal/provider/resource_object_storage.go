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
	"github.com/zsoftly/zcp-cli/pkg/api/objectstorage"
)

var _ resource.Resource = &objectStorageResource{}
var _ resource.ResourceWithConfigure = &objectStorageResource{}
var _ resource.ResourceWithImportState = &objectStorageResource{}
var _ resource.ResourceWithValidateConfig = &objectStorageResource{}

// objectStorageServiceIface is shared by zcp_object_storage and
// zcp_object_storage_bucket.
type objectStorageServiceIface interface {
	Get(ctx context.Context, slug string) (*objectstorage.ObjectStorage, error)
	Create(ctx context.Context, req objectstorage.CreateRequest) (*objectstorage.ObjectStorage, error)
	Delete(ctx context.Context, slug string) error
	Resize(ctx context.Context, slug string, storageGB int) (*objectstorage.ObjectStorage, error)
	GetBucket(ctx context.Context, slug, bucketSlug string) (*objectstorage.Bucket, error)
	CreateBucket(ctx context.Context, slug, name string) (*objectstorage.Bucket, error)
	DeleteBucket(ctx context.Context, slug, bucketSlug string) error
}

type objectStorageResource struct {
	svc            objectStorageServiceIface
	defaultProject string
}

type objectStorageResourceModel struct {
	ID              types.String   `tfsdk:"id"`
	Name            types.String   `tfsdk:"name"`
	CloudProvider   types.String   `tfsdk:"cloud_provider"`
	Region          types.String   `tfsdk:"region"`
	Project         types.String   `tfsdk:"project"`
	BillingCycle    types.String   `tfsdk:"billing_cycle"`
	StorageCategory types.String   `tfsdk:"storage_category"`
	Plan            types.String   `tfsdk:"plan"`
	SizeGB          types.Int64    `tfsdk:"size_gb"`
	Status          types.String   `tfsdk:"status"`
	Size            types.Int64    `tfsdk:"size"`
	APIKey          types.String   `tfsdk:"api_key"`
	APISecret       types.String   `tfsdk:"api_secret"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
}

func NewObjectStorageResource() resource.Resource {
	return &objectStorageResource{}
}

func (r *objectStorageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_object_storage"
}

func (r *objectStorageResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP object storage store (S3-compatible). Create buckets with " +
			"`zcp_object_storage_bucket`. `size_gb` resizes in place; every other change forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Object storage slug.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name for the store. Changing this forces replacement.",
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
			"billing_cycle": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Billing cycle (e.g. `hourly`, `monthly`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"storage_category": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Storage category slug (region-specific, e.g. `nvme`, `pro-nvme`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"plan": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Catalogue plan slug. Exactly one of `plan` or `size_gb` must be set. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"size_gb": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Custom store size in GB. Exactly one of `plan` or `size_gb` must be set. Increasing it resizes the store in place.",
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current status of the store.",
			},
			"size": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Provisioned size in GB as reported by the API.",
			},
			"api_key": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "S3 access key for the store.",
			},
			"api_secret": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "S3 secret key for the store.",
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

func (r *objectStorageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = objectstorage.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

// ValidateConfig enforces exactly one of plan / size_gb.
func (r *objectStorageResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var model objectStorageResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// An unknown value (e.g. from a variable or another resource) resolves at
	// apply time, so exclusivity cannot be judged yet; skip rather than fail.
	if model.Plan.IsUnknown() || model.SizeGB.IsUnknown() {
		return
	}
	planSet := !model.Plan.IsNull()
	sizeSet := !model.SizeGB.IsNull()
	if planSet == sizeSet {
		resp.Diagnostics.AddError(
			"Invalid plan configuration",
			"Set exactly one of `plan` (catalogue plan) or `size_gb` (custom size).",
		)
	}
}

// applyStoreState populates computed attributes from an ObjectStorage.
func applyStoreState(model *objectStorageResourceModel, store *objectstorage.ObjectStorage) {
	model.ID = types.StringValue(store.Slug)
	model.Status = types.StringValue(store.Status)
	if size, err := store.Size.Int64(); err == nil {
		model.Size = types.Int64Value(size)
	} else {
		model.Size = types.Int64Null()
	}
	if store.APIKey != "" {
		model.APIKey = types.StringValue(store.APIKey)
	} else if model.APIKey.IsUnknown() {
		model.APIKey = types.StringNull()
	}
	if store.APISecret != "" {
		model.APISecret = types.StringValue(store.APISecret)
	} else if model.APISecret.IsUnknown() {
		model.APISecret = types.StringNull()
	}
}

func (r *objectStorageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model objectStorageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_object_storage cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 10*time.Minute)
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

	createReq := objectstorage.CreateRequest{
		Name:            model.Name.ValueString(),
		Project:         project,
		CloudProvider:   model.CloudProvider.ValueString(),
		Region:          model.Region.ValueString(),
		BillingCycle:    model.BillingCycle.ValueString(),
		StorageCategory: model.StorageCategory.ValueString(),
	}
	if !model.Plan.IsNull() && !model.Plan.IsUnknown() {
		createReq.Plan = model.Plan.ValueString()
	} else {
		createReq.CustomPlan = &objectstorage.CustomPlan{Storage: int(model.SizeGB.ValueInt64())}
	}

	store, err := r.svc.Create(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create object storage", err.Error())
		return
	}

	// The create response can omit credentials; refresh from Get when needed.
	if store.APIKey == "" || store.APISecret == "" {
		if full, gerr := r.svc.Get(ctx, store.Slug); gerr == nil {
			full.Status = firstNonEmpty(full.Status, store.Status)
			store = full
		}
	}

	applyStoreState(&model, store)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// firstNonEmpty returns a when non-empty, else b.
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (r *objectStorageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model objectStorageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_object_storage cannot be read: bearer_token is missing.")
		return
	}

	store, err := r.svc.Get(ctx, model.ID.ValueString())
	if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read object storage", err.Error())
		return
	}

	model.Name = types.StringValue(store.Name)
	applyStoreState(&model, store)
	// size_gb tracks the API-reported size for custom stores so out-of-band
	// resizes surface as drift; plan-based stores keep size_gb null.
	if !model.SizeGB.IsNull() && !model.Size.IsNull() {
		model.SizeGB = model.Size
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *objectStorageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state objectStorageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_object_storage cannot be updated: bearer_token is missing.")
		return
	}

	updateTimeout, diags := plan.Timeouts.Update(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	slug := state.ID.ValueString()
	model := plan
	model.ID = state.ID
	model.APIKey = state.APIKey
	model.APISecret = state.APISecret
	model.Status = state.Status
	model.Size = state.Size

	if !plan.SizeGB.IsNull() && !plan.SizeGB.IsUnknown() && plan.SizeGB.ValueInt64() != state.SizeGB.ValueInt64() {
		store, err := r.svc.Resize(ctx, slug, int(plan.SizeGB.ValueInt64()))
		if err != nil {
			resp.Diagnostics.AddError("Failed to resize object storage", err.Error())
			return
		}
		if store != nil && store.Slug != "" {
			applyStoreState(&model, store)
		} else if full, gerr := r.svc.Get(ctx, slug); gerr == nil {
			applyStoreState(&model, full)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *objectStorageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model objectStorageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_object_storage cannot be deleted: bearer_token is missing.")
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
		resp.Diagnostics.AddError("Failed to delete object storage", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		_, err := r.svc.Get(ctx, slug)
		if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
			return false, nil
		}
		return err == nil, err
	}); err != nil {
		resp.Diagnostics.AddError("Object storage deletion did not complete", err.Error())
	}
}

// ImportState accepts a composite ID so the write-only create attributes are
// seeded for a zero-diff plan after import. Format:
//
//	<slug>/<cloud_provider>/<region>/<billing_cycle>/<storage_category>[/<plan>/<project>]
//
// name, status, size, and credentials come from the subsequent Read.
func (r *objectStorageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"id", "cloud_provider", "region", "billing_cycle", "storage_category", "plan", "project"}
	importPositional(ctx, req, resp, fields, 5,
		"<slug>/<cloud_provider>/<region>/<billing_cycle>/<storage_category>[/<plan>/<project>]")
}
