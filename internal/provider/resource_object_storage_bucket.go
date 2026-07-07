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
	"github.com/zsoftly/zcp-cli/pkg/api/objectstorage"
)

var _ resource.Resource = &objectStorageBucketResource{}
var _ resource.ResourceWithConfigure = &objectStorageBucketResource{}
var _ resource.ResourceWithImportState = &objectStorageBucketResource{}

type objectStorageBucketResource struct {
	svc objectStorageServiceIface
}

type objectStorageBucketResourceModel struct {
	ID            types.String   `tfsdk:"id"`
	ObjectStorage types.String   `tfsdk:"object_storage"`
	Name          types.String   `tfsdk:"name"`
	Status        types.String   `tfsdk:"status"`
	Timeouts      timeouts.Value `tfsdk:"timeouts"`
}

func NewObjectStorageBucketResource() resource.Resource {
	return &objectStorageBucketResource{}
}

func (r *objectStorageBucketResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_object_storage_bucket"
}

func (r *objectStorageBucketResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a bucket in a `zcp_object_storage` store. Bucket contents, policies, and " +
			"lifecycle settings are managed via the S3 API (e.g. the AWS/minio providers pointed at the store's " +
			"endpoint with its `api_key`/`api_secret`), not by this resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Bucket slug.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"object_storage": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Parent object storage slug. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Bucket name. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current bucket status.",
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

func (r *objectStorageBucketResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *objectStorageBucketResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model objectStorageBucketResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_object_storage_bucket cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	bucket, err := r.svc.CreateBucket(ctx, model.ObjectStorage.ValueString(), model.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create bucket", err.Error())
		return
	}
	if bucket == nil || bucket.Slug == "" {
		resp.Diagnostics.AddError("Failed to create bucket",
			fmt.Sprintf("the API accepted the create for %q but returned no bucket; check the bucket list before retrying.", model.Name.ValueString()))
		return
	}

	model.ID = types.StringValue(bucket.Slug)
	if bucket.Status != "" {
		model.Status = types.StringValue(bucket.Status)
	} else {
		model.Status = types.StringNull()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *objectStorageBucketResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model objectStorageBucketResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_object_storage_bucket cannot be read: bearer_token is missing.")
		return
	}

	bucket, err := r.svc.GetBucket(ctx, model.ObjectStorage.ValueString(), model.ID.ValueString())
	if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read bucket", err.Error())
		return
	}

	model.Name = types.StringValue(bucket.Name)
	if bucket.Status != "" {
		model.Status = types.StringValue(bucket.Status)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *objectStorageBucketResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are ForceNew; Terraform never invokes this method.
}

func (r *objectStorageBucketResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model objectStorageBucketResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_object_storage_bucket cannot be deleted: bearer_token is missing.")
		return
	}

	deleteTimeout, diags := model.Timeouts.Delete(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	storeSlug := model.ObjectStorage.ValueString()
	bucketSlug := model.ID.ValueString()
	err := r.svc.DeleteBucket(deleteCtx, storeSlug, bucketSlug)
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete bucket", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		_, err := r.svc.GetBucket(ctx, storeSlug, bucketSlug)
		if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
			return false, nil
		}
		return err == nil, err
	}); err != nil {
		resp.Diagnostics.AddError("Bucket deletion did not complete", err.Error())
	}
}

func (r *objectStorageBucketResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Expected format: "store-slug/bucket-slug"
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected format \"<object_storage>/<bucket_slug>\", got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("object_storage"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
