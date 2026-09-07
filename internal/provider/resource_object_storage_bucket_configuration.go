package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/objectstorage"
)

// Bucket settings use the S3-compatible gateway. These resources do not expose
// or retain the gateway access key or secret in their own resource state.
type bucketConfigurationResource struct {
	svc  objectStorageServiceIface
	kind string
}

type bucketConfigurationModel struct {
	ID                 types.String `tfsdk:"id"`
	ObjectStorage      types.String `tfsdk:"object_storage"`
	Bucket             types.String `tfsdk:"bucket"`
	Enabled            types.Bool   `tfsdk:"enabled"`
	Policy             types.String `tfsdk:"policy"`
	Tags               types.Map    `tfsdk:"tags"`
	Prefix             types.String `tfsdk:"prefix"`
	Days               types.Int64  `tfsdk:"days"`
	NoncurrentDays     types.Int64  `tfsdk:"noncurrent_days"`
	AbortMultipartDays types.Int64  `tfsdk:"abort_incomplete_multipart_upload_days"`
	AllowedOrigins     types.List   `tfsdk:"allowed_origins"`
	AllowedMethods     types.List   `tfsdk:"allowed_methods"`
	AllowedHeaders     types.List   `tfsdk:"allowed_headers"`
	MaxAgeSeconds      types.Int64  `tfsdk:"max_age_seconds"`
}

func NewObjectStorageBucketVersioningResource() resource.Resource {
	return &bucketConfigurationResource{kind: "versioning"}
}
func NewObjectStorageBucketPolicyResource() resource.Resource {
	return &bucketConfigurationResource{kind: "policy"}
}
func NewObjectStorageBucketTaggingResource() resource.Resource {
	return &bucketConfigurationResource{kind: "tagging"}
}
func NewObjectStorageBucketLifecycleResource() resource.Resource {
	return &bucketConfigurationResource{kind: "lifecycle"}
}
func NewObjectStorageBucketCORSResource() resource.Resource {
	return &bucketConfigurationResource{kind: "cors"}
}

func (r *bucketConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_object_storage_bucket_" + r.kind
}

func bucketConfigurationAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Composite object storage and bucket identifier.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"object_storage": schema.StringAttribute{Required: true, MarkdownDescription: "Object storage slug. Changing this forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"bucket":         schema.StringAttribute{Required: true, MarkdownDescription: "Bucket slug. Changing this forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
	}
}

func (r *bucketConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	a := bucketConfigurationAttributes()
	// The shared model has one field set for every configuration type. Attributes
	// outside this resource's concern are computed-only and remain null.
	a["enabled"] = schema.BoolAttribute{Computed: true}
	a["policy"] = schema.StringAttribute{Computed: true}
	a["tags"] = schema.MapAttribute{Computed: true, ElementType: types.StringType}
	a["prefix"] = schema.StringAttribute{Computed: true}
	a["days"] = schema.Int64Attribute{Computed: true}
	a["noncurrent_days"] = schema.Int64Attribute{Computed: true}
	a["abort_incomplete_multipart_upload_days"] = schema.Int64Attribute{Computed: true}
	a["allowed_origins"] = schema.ListAttribute{Computed: true, ElementType: types.StringType}
	a["allowed_methods"] = schema.ListAttribute{Computed: true, ElementType: types.StringType}
	a["allowed_headers"] = schema.ListAttribute{Computed: true, ElementType: types.StringType}
	a["max_age_seconds"] = schema.Int64Attribute{Computed: true}
	switch r.kind {
	case "versioning":
		a["enabled"] = schema.BoolAttribute{Required: true, MarkdownDescription: "Whether object versioning is enabled.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}}
	case "policy":
		a["policy"] = schema.StringAttribute{Required: true, MarkdownDescription: "S3 bucket policy JSON. An empty string removes the policy."}
	case "tagging":
		a["tags"] = schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Complete set of bucket tags."}
	case "lifecycle":
		a["prefix"] = schema.StringAttribute{Optional: true, Default: stringdefault.StaticString(""), MarkdownDescription: "Object-key prefix covered by the lifecycle rule."}
		a["days"] = schema.Int64Attribute{Optional: true, MarkdownDescription: "Days before current objects expire."}
		a["noncurrent_days"] = schema.Int64Attribute{Optional: true, MarkdownDescription: "Days before noncurrent object versions expire."}
		a["abort_incomplete_multipart_upload_days"] = schema.Int64Attribute{Optional: true, MarkdownDescription: "Days before incomplete multipart uploads are aborted."}
	case "cors":
		a["allowed_origins"] = schema.ListAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Allowed CORS origins."}
		a["allowed_methods"] = schema.ListAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Allowed CORS methods."}
		a["allowed_headers"] = schema.ListAttribute{Optional: true, ElementType: types.StringType, MarkdownDescription: "Allowed CORS request headers."}
		a["max_age_seconds"] = schema.Int64Attribute{Optional: true, Default: int64default.StaticInt64(0), MarkdownDescription: "CORS preflight cache duration in seconds."}
	}
	resp.Schema = schema.Schema{MarkdownDescription: "Manages one bucket configuration through the object storage S3-compatible gateway.", Attributes: a}
}

func (r *bucketConfigurationResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m bucketConfigurationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	switch r.kind {
	case "lifecycle":
		if m.Days.IsUnknown() || m.NoncurrentDays.IsUnknown() || m.AbortMultipartDays.IsUnknown() {
			return
		}
		if m.Days.ValueInt64() <= 0 && m.NoncurrentDays.ValueInt64() <= 0 && m.AbortMultipartDays.ValueInt64() <= 0 {
			resp.Diagnostics.AddError("Invalid lifecycle configuration", "Set at least one lifecycle duration greater than zero.")
		}
	case "cors":
		if m.AllowedOrigins.IsUnknown() || m.AllowedMethods.IsUnknown() {
			return
		}
		if len(m.AllowedOrigins.Elements()) == 0 || len(m.AllowedMethods.Elements()) == 0 {
			resp.Diagnostics.AddError("Invalid CORS configuration", "Set at least one allowed origin and one allowed method.")
		}
	case "policy":
		if !m.Policy.IsUnknown() && !json.Valid([]byte(m.Policy.ValueString())) {
			resp.Diagnostics.AddError("Invalid bucket policy", "Set `policy` to valid JSON.")
		}
	case "tagging":
		if !m.Tags.IsUnknown() && len(m.Tags.Elements()) == 0 {
			resp.Diagnostics.AddError("Invalid bucket tags", "Set at least one tag. Remove this resource to delete all bucket tags.")
		}
	}
}

func (r *bucketConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *bucketConfigurationResource) bucketName(ctx context.Context, store, bucket string) (string, error) {
	b, err := r.svc.GetBucket(ctx, store, bucket)
	if err != nil {
		return "", err
	}
	if b == nil || b.Name == "" {
		return "", fmt.Errorf("bucket %q returned no S3 bucket name", bucket)
	}
	return b.Name, nil
}

func (r *bucketConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m bucketConfigurationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "Object storage bucket configuration cannot be created: bearer_token is missing.")
		return
	}
	name, err := r.bucketName(ctx, m.ObjectStorage.ValueString(), m.Bucket.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to find bucket", err.Error())
		return
	}
	if err := r.apply(ctx, &m, name); err != nil {
		resp.Diagnostics.AddError("Failed to configure bucket", err.Error())
		return
	}
	m.ID = types.StringValue(m.ObjectStorage.ValueString() + "/" + m.Bucket.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *bucketConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m bucketConfigurationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "Object storage bucket configuration cannot be read: bearer_token is missing.")
		return
	}
	name, err := r.bucketName(ctx, m.ObjectStorage.ValueString(), m.Bucket.ValueString())
	if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read bucket configuration", err.Error())
		return
	}
	if err := r.read(ctx, &m, name); err != nil {
		resp.Diagnostics.AddError("Failed to read bucket configuration", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *bucketConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m bucketConfigurationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name, err := r.bucketName(ctx, m.ObjectStorage.ValueString(), m.Bucket.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to find bucket", err.Error())
		return
	}
	if err := r.apply(ctx, &m, name); err != nil {
		resp.Diagnostics.AddError("Failed to configure bucket", err.Error())
		return
	}
	m.ID = types.StringValue(m.ObjectStorage.ValueString() + "/" + m.Bucket.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *bucketConfigurationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m bucketConfigurationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "Object storage bucket configuration cannot be deleted: bearer_token is missing.")
		return
	}
	name, err := r.bucketName(ctx, m.ObjectStorage.ValueString(), m.Bucket.ValueString())
	if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to find bucket", err.Error())
		return
	}
	var deleteErr error
	switch r.kind {
	case "versioning":
		deleteErr = r.svc.SetBucketVersioning(ctx, m.ObjectStorage.ValueString(), name, false)
	case "policy":
		deleteErr = r.svc.PutBucketPolicy(ctx, m.ObjectStorage.ValueString(), name, "")
	case "tagging":
		deleteErr = r.svc.DeleteBucketTagging(ctx, m.ObjectStorage.ValueString(), name)
	case "lifecycle":
		deleteErr = r.svc.DeleteBucketLifecycle(ctx, m.ObjectStorage.ValueString(), name)
	case "cors":
		deleteErr = r.svc.DeleteBucketCORS(ctx, m.ObjectStorage.ValueString(), name)
	}
	if deleteErr != nil {
		resp.Diagnostics.AddError("Failed to remove bucket configuration", deleteErr.Error())
	}
}

func (r *bucketConfigurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected format \"<object_storage>/<bucket_slug>\", got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("object_storage"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("bucket"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *bucketConfigurationResource) apply(ctx context.Context, m *bucketConfigurationModel, name string) error {
	store := m.ObjectStorage.ValueString()
	switch r.kind {
	case "versioning":
		return r.svc.SetBucketVersioning(ctx, store, name, m.Enabled.ValueBool())
	case "policy":
		return r.svc.PutBucketPolicy(ctx, store, name, m.Policy.ValueString())
	case "tagging":
		v := map[string]string{}
		if !m.Tags.IsNull() {
			if d := m.Tags.ElementsAs(ctx, &v, false); d.HasError() {
				return fmt.Errorf("reading tags: %s", d.Errors()[0].Summary())
			}
		}
		return r.svc.SetBucketTagging(ctx, store, name, v)
	case "lifecycle":
		return r.svc.SetBucketExpiry(ctx, store, name, m.Prefix.ValueString(), int(m.Days.ValueInt64()), int(m.NoncurrentDays.ValueInt64()), int(m.AbortMultipartDays.ValueInt64()))
	case "cors":
		origins, err := stringList(ctx, m.AllowedOrigins)
		if err != nil {
			return err
		}
		methods, err := stringList(ctx, m.AllowedMethods)
		if err != nil {
			return err
		}
		headers, err := stringList(ctx, m.AllowedHeaders)
		if err != nil {
			return err
		}
		return r.svc.SetBucketCORS(ctx, store, name, origins, methods, headers, int(m.MaxAgeSeconds.ValueInt64()))
	}
	return fmt.Errorf("unsupported bucket configuration %q", r.kind)
}

func (r *bucketConfigurationResource) read(ctx context.Context, m *bucketConfigurationModel, name string) error {
	store := m.ObjectStorage.ValueString()
	switch r.kind {
	case "versioning":
		status, err := r.svc.GetBucketVersioning(ctx, store, name)
		if err != nil {
			return err
		}
		m.Enabled = types.BoolValue(strings.EqualFold(status, "Enabled"))
	case "policy":
		policy, err := r.svc.GetBucketPolicy(ctx, store, name)
		if err != nil {
			return err
		}
		m.Policy = types.StringValue(policy)
	case "tagging":
		tags, err := r.svc.GetBucketTagging(ctx, store, name)
		if err != nil {
			return err
		}
		v, d := types.MapValueFrom(ctx, types.StringType, tags)
		if d.HasError() {
			return fmt.Errorf("reading tags: %s", d.Errors()[0].Summary())
		}
		m.Tags = v
	case "lifecycle":
		raw, err := r.svc.GetBucketLifecycle(ctx, store, name)
		if err != nil {
			return err
		}
		return decodeLifecycle(raw, m)
	case "cors":
		raw, err := r.svc.GetBucketCORS(ctx, store, name)
		if err != nil {
			return err
		}
		return decodeCORS(ctx, raw, m)
	}
	return nil
}

func stringList(ctx context.Context, v types.List) ([]string, error) {
	if v.IsNull() {
		return nil, nil
	}
	var out []string
	if d := v.ElementsAs(ctx, &out, false); d.HasError() {
		return nil, fmt.Errorf("reading list: %s", d.Errors()[0].Summary())
	}
	return out, nil
}

func decodeLifecycle(raw string, m *bucketConfigurationModel) error {
	if raw == "" {
		m.Prefix = types.StringNull()
		m.Days = types.Int64Null()
		m.NoncurrentDays = types.Int64Null()
		m.AbortMultipartDays = types.Int64Null()
		return nil
	}
	var cfg struct {
		Rules []struct {
			Filter struct {
				Prefix string `json:"Prefix"`
			} `json:"Filter"`
			Expiration *struct {
				Days int64 `json:"Days"`
			} `json:"Expiration"`
			Noncurrent *struct {
				Days int64 `json:"NoncurrentDays"`
			} `json:"NoncurrentVersionExpiration"`
			Abort *struct {
				Days int64 `json:"DaysAfterInitiation"`
			} `json:"AbortIncompleteMultipartUpload"`
		} `json:"Rules"`
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return fmt.Errorf("decoding lifecycle configuration: %w", err)
	}
	if len(cfg.Rules) == 0 {
		return nil
	}
	rule := cfg.Rules[0]
	m.Prefix = types.StringValue(rule.Filter.Prefix)
	if rule.Expiration != nil {
		m.Days = types.Int64Value(rule.Expiration.Days)
	} else {
		m.Days = types.Int64Null()
	}
	if rule.Noncurrent != nil {
		m.NoncurrentDays = types.Int64Value(rule.Noncurrent.Days)
	} else {
		m.NoncurrentDays = types.Int64Null()
	}
	if rule.Abort != nil {
		m.AbortMultipartDays = types.Int64Value(rule.Abort.Days)
	} else {
		m.AbortMultipartDays = types.Int64Null()
	}
	return nil
}

func decodeCORS(ctx context.Context, raw string, m *bucketConfigurationModel) error {
	if raw == "" {
		m.AllowedOrigins = types.ListNull(types.StringType)
		m.AllowedMethods = types.ListNull(types.StringType)
		m.AllowedHeaders = types.ListNull(types.StringType)
		m.MaxAgeSeconds = types.Int64Null()
		return nil
	}
	var rules []struct {
		AllowedOrigin []string `json:"AllowedOrigin"`
		AllowedMethod []string `json:"AllowedMethod"`
		AllowedHeader []string `json:"AllowedHeader"`
		MaxAgeSeconds int64    `json:"MaxAgeSeconds"`
	}
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return fmt.Errorf("decoding CORS configuration: %w", err)
	}
	if len(rules) == 0 {
		return nil
	}
	rule := rules[0]
	origins, d := types.ListValueFrom(ctx, types.StringType, rule.AllowedOrigin)
	if d.HasError() {
		return fmt.Errorf("reading CORS origins: %s", d.Errors()[0].Summary())
	}
	methods, d := types.ListValueFrom(ctx, types.StringType, rule.AllowedMethod)
	if d.HasError() {
		return fmt.Errorf("reading CORS methods: %s", d.Errors()[0].Summary())
	}
	headers, d := types.ListValueFrom(ctx, types.StringType, rule.AllowedHeader)
	if d.HasError() {
		return fmt.Errorf("reading CORS headers: %s", d.Errors()[0].Summary())
	}
	m.AllowedOrigins, m.AllowedMethods, m.AllowedHeaders, m.MaxAgeSeconds = origins, methods, headers, types.Int64Value(rule.MaxAgeSeconds)
	return nil
}

var _ resource.Resource = &bucketConfigurationResource{}
var _ resource.ResourceWithConfigure = &bucketConfigurationResource{}
var _ resource.ResourceWithImportState = &bucketConfigurationResource{}
var _ resource.ResourceWithValidateConfig = &bucketConfigurationResource{}
