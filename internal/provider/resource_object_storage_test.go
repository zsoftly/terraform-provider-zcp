package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/objectstorage"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeObjectStorageService satisfies objectStorageServiceIface.
type fakeObjectStorageService struct {
	store          *objectstorage.ObjectStorage
	created        *objectstorage.ObjectStorage
	bucket         *objectstorage.Bucket
	createdBucket  *objectstorage.Bucket
	err            error
	getErr         error
	deleted        []string
	resizedTo      []int
	deletedBuckets []string
	versioning     string
	policy         string
	tags           map[string]string
	lifecycle      string
	cors           string
}

func (f *fakeObjectStorageService) Get(_ context.Context, _ string) (*objectstorage.ObjectStorage, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.store == nil {
		return nil, &apierrors.APIError{StatusCode: 404, Message: "not found"}
	}
	return f.store, f.err
}
func (f *fakeObjectStorageService) Create(_ context.Context, _ objectstorage.CreateRequest) (*objectstorage.ObjectStorage, error) {
	return f.created, f.err
}
func (f *fakeObjectStorageService) Delete(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	if f.err == nil {
		f.store = nil
	}
	return f.err
}
func (f *fakeObjectStorageService) Resize(_ context.Context, _ string, storageGB int) (*objectstorage.ObjectStorage, error) {
	f.resizedTo = append(f.resizedTo, storageGB)
	if f.err == nil && f.store != nil {
		f.store.Size = json.Number(itoa(storageGB))
		return f.store, nil
	}
	return nil, f.err
}
func (f *fakeObjectStorageService) GetBucket(_ context.Context, _, _ string) (*objectstorage.Bucket, error) {
	if f.bucket == nil {
		return nil, &apierrors.APIError{StatusCode: 404, Message: "not found"}
	}
	return f.bucket, f.err
}
func (f *fakeObjectStorageService) CreateBucket(_ context.Context, _, _ string) (*objectstorage.Bucket, error) {
	return f.createdBucket, f.err
}
func (f *fakeObjectStorageService) DeleteBucket(_ context.Context, _, bucketSlug string) error {
	f.deletedBuckets = append(f.deletedBuckets, bucketSlug)
	if f.err == nil {
		f.bucket = nil
	}
	return f.err
}
func (f *fakeObjectStorageService) SetBucketVersioning(_ context.Context, _, _ string, enabled bool) error {
	if enabled {
		f.versioning = "Enabled"
	} else {
		f.versioning = "Suspended"
	}
	return f.err
}
func (f *fakeObjectStorageService) GetBucketVersioning(_ context.Context, _, _ string) (string, error) {
	return f.versioning, f.err
}
func (f *fakeObjectStorageService) GetBucketPolicy(_ context.Context, _, _ string) (string, error) {
	return f.policy, f.err
}
func (f *fakeObjectStorageService) PutBucketPolicy(_ context.Context, _, _ string, policy string) error {
	f.policy = policy
	return f.err
}
func (f *fakeObjectStorageService) GetBucketTagging(_ context.Context, _, _ string) (map[string]string, error) {
	return f.tags, f.err
}
func (f *fakeObjectStorageService) SetBucketTagging(_ context.Context, _, _ string, tags map[string]string) error {
	f.tags = tags
	return f.err
}
func (f *fakeObjectStorageService) DeleteBucketTagging(_ context.Context, _, _ string) error {
	f.tags = map[string]string{}
	return f.err
}
func (f *fakeObjectStorageService) SetBucketExpiry(_ context.Context, _, _ string, _ string, _, _, _ int) error {
	return f.err
}
func (f *fakeObjectStorageService) GetBucketLifecycle(_ context.Context, _, _ string) (string, error) {
	return f.lifecycle, f.err
}
func (f *fakeObjectStorageService) DeleteBucketLifecycle(_ context.Context, _, _ string) error {
	f.lifecycle = ""
	return f.err
}
func (f *fakeObjectStorageService) SetBucketCORS(_ context.Context, _, _ string, _, _, _ []string, _ int) error {
	return f.err
}
func (f *fakeObjectStorageService) GetBucketCORS(_ context.Context, _, _ string) (string, error) {
	return f.cors, f.err
}
func (f *fakeObjectStorageService) DeleteBucketCORS(_ context.Context, _, _ string) error {
	f.cors = ""
	return f.err
}

func itoa(n int) string {
	return json.Number(types.Int64Value(int64(n)).String()).String()
}

// --- zcp_object_storage ---

type objectStorageStateModel struct {
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

func objectStorageSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewObjectStorageResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func objectStorageRaw(t *testing.T, schResp resource.SchemaResponse, id string, sizeGB *int64, size *int64) tftypes.Value {
	t.Helper()
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	str := func(v string) tftypes.Value {
		if v == "" {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, v)
	}
	num := func(v *int64) tftypes.Value {
		if v == nil {
			return tftypes.NewValue(tftypes.Number, nil)
		}
		return tftypes.NewValue(tftypes.Number, *v)
	}
	return tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               str(id),
		"name":             str("assets"),
		"cloud_provider":   str("zsoftly"),
		"region":           str("yow-1"),
		"project":          tftypes.NewValue(tftypes.String, nil),
		"billing_cycle":    str("hourly"),
		"storage_category": str("nvme"),
		"plan":             tftypes.NewValue(tftypes.String, nil),
		"size_gb":          num(sizeGB),
		"status":           tftypes.NewValue(tftypes.String, nil),
		"size":             num(size),
		"api_key":          tftypes.NewValue(tftypes.String, nil),
		"api_secret":       tftypes.NewValue(tftypes.String, nil),
		"timeouts":         timeoutsNull(t, schResp),
	})
}

func TestObjectStorageResource_createHappyPath(t *testing.T) {
	svc := &fakeObjectStorageService{
		created: &objectstorage.ObjectStorage{
			Slug:      "assets-x1",
			Name:      "assets",
			Status:    "Active",
			Size:      "100",
			APIKey:    "AK",
			APISecret: "SK",
		},
	}
	r := internalprovider.NewObjectStorageResourceWithService(svc)
	schResp := objectStorageSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	size := int64(100)
	createReq := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: objectStorageRaw(t, schResp, "", &size, nil)},
	}
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)},
	}
	r.Create(context.Background(), createReq, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", createResp.Diagnostics)
	}
	var got objectStorageStateModel
	if diags := createResp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "assets-x1" {
		t.Errorf("ID = %q, want assets-x1", got.ID.ValueString())
	}
	if got.APIKey.ValueString() != "AK" || got.APISecret.ValueString() != "SK" {
		t.Errorf("credentials = %q/%q, want AK/SK", got.APIKey.ValueString(), got.APISecret.ValueString())
	}
	if got.Size.ValueInt64() != 100 {
		t.Errorf("Size = %d, want 100", got.Size.ValueInt64())
	}
}

func TestObjectStorageResource_createServiceError(t *testing.T) {
	svc := &fakeObjectStorageService{err: errors.New("quota exceeded")}
	r := internalprovider.NewObjectStorageResourceWithService(svc)
	schResp := objectStorageSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	size := int64(100)
	createReq := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: objectStorageRaw(t, schResp, "", &size, nil)},
	}
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)},
	}
	r.Create(context.Background(), createReq, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func TestObjectStorageResource_readNotFoundRemoves(t *testing.T) {
	svc := &fakeObjectStorageService{} // store nil → Get 404s
	r := internalprovider.NewObjectStorageResourceWithService(svc)
	schResp := objectStorageSchema(t)
	size := int64(100)
	stateVal := objectStorageRaw(t, schResp, "assets-x1", &size, &size)
	readReq := resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), readReq, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics)
	}
	if !readResp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestObjectStorageResource_updateResizes(t *testing.T) {
	svc := &fakeObjectStorageService{
		store: &objectstorage.ObjectStorage{Slug: "assets-x1", Name: "assets", Status: "Active", Size: "100"},
	}
	r := internalprovider.NewObjectStorageResourceWithService(svc)
	schResp := objectStorageSchema(t)
	oldSize, newSize := int64(100), int64(200)
	updateReq := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schResp.Schema, Raw: objectStorageRaw(t, schResp, "assets-x1", &newSize, &oldSize)},
		State: tfsdk.State{Schema: schResp.Schema, Raw: objectStorageRaw(t, schResp, "assets-x1", &oldSize, &oldSize)},
	}
	updateResp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: objectStorageRaw(t, schResp, "assets-x1", &oldSize, &oldSize)},
	}
	r.Update(context.Background(), updateReq, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", updateResp.Diagnostics)
	}
	if len(svc.resizedTo) != 1 || svc.resizedTo[0] != 200 {
		t.Errorf("Resize called with %v, want [200]", svc.resizedTo)
	}
	var got objectStorageStateModel
	if diags := updateResp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Size.ValueInt64() != 200 {
		t.Errorf("Size after resize = %d, want 200", got.Size.ValueInt64())
	}
}

func TestObjectStorageResource_deleteHappyPath(t *testing.T) {
	svc := &fakeObjectStorageService{
		store: &objectstorage.ObjectStorage{Slug: "assets-x1"},
	}
	r := internalprovider.NewObjectStorageResourceWithService(svc)
	schResp := objectStorageSchema(t)
	size := int64(100)
	stateVal := objectStorageRaw(t, schResp, "assets-x1", &size, &size)
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", deleteResp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "assets-x1" {
		t.Errorf("Delete called with %v, want [assets-x1]", svc.deleted)
	}
}

// --- zcp_object_storage_bucket ---

type bucketStateModel struct {
	ID            types.String   `tfsdk:"id"`
	ObjectStorage types.String   `tfsdk:"object_storage"`
	Name          types.String   `tfsdk:"name"`
	Status        types.String   `tfsdk:"status"`
	Timeouts      timeouts.Value `tfsdk:"timeouts"`
}

func bucketSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewObjectStorageBucketResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func bucketRaw(t *testing.T, schResp resource.SchemaResponse, id, store, name string) tftypes.Value {
	t.Helper()
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	str := func(v string) tftypes.Value {
		if v == "" {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, v)
	}
	return tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             str(id),
		"object_storage": str(store),
		"name":           str(name),
		"status":         tftypes.NewValue(tftypes.String, nil),
		"timeouts":       timeoutsNull(t, schResp),
	})
}

func TestObjectStorageBucketResource_createHappyPath(t *testing.T) {
	svc := &fakeObjectStorageService{
		createdBucket: &objectstorage.Bucket{Slug: "media-b1", Name: "media", Status: "Active"},
	}
	r := internalprovider.NewObjectStorageBucketResourceWithService(svc)
	schResp := bucketSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	createReq := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: bucketRaw(t, schResp, "", "assets-x1", "media")},
	}
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)},
	}
	r.Create(context.Background(), createReq, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", createResp.Diagnostics)
	}
	var got bucketStateModel
	if diags := createResp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "media-b1" {
		t.Errorf("ID = %q, want media-b1", got.ID.ValueString())
	}
}

func TestObjectStorageBucketResource_readNotFoundRemoves(t *testing.T) {
	svc := &fakeObjectStorageService{} // bucket nil → GetBucket 404s
	r := internalprovider.NewObjectStorageBucketResourceWithService(svc)
	schResp := bucketSchema(t)
	stateVal := bucketRaw(t, schResp, "media-b1", "assets-x1", "media")
	readReq := resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), readReq, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics)
	}
	if !readResp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestObjectStorageBucketResource_deleteHappyPath(t *testing.T) {
	svc := &fakeObjectStorageService{
		bucket: &objectstorage.Bucket{Slug: "media-b1", Name: "media"},
	}
	r := internalprovider.NewObjectStorageBucketResourceWithService(svc)
	schResp := bucketSchema(t)
	stateVal := bucketRaw(t, schResp, "media-b1", "assets-x1", "media")
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", deleteResp.Diagnostics)
	}
	if len(svc.deletedBuckets) != 1 || svc.deletedBuckets[0] != "media-b1" {
		t.Errorf("DeleteBucket called with %v, want [media-b1]", svc.deletedBuckets)
	}
}

func TestObjectStorageResource_validateConfig(t *testing.T) {
	r := internalprovider.NewObjectStorageResource().(resource.ResourceWithValidateConfig)
	var schResp resource.SchemaResponse
	r.(resource.Resource).Schema(context.Background(), resource.SchemaRequest{}, &schResp)

	run := func(planSlug string, sizeGB tftypes.Value) resource.ValidateConfigResponse {
		raw := objectStorageRaw(t, schResp, "", nil, nil)
		vals := map[string]tftypes.Value{}
		if err := raw.As(&vals); err != nil {
			t.Fatalf("decomposing raw: %v", err)
		}
		if planSlug != "" {
			vals["plan"] = tftypes.NewValue(tftypes.String, planSlug)
		}
		vals["size_gb"] = sizeGB
		tfType := schResp.Schema.Type().TerraformType(context.Background())
		req := resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, vals)}}
		var resp resource.ValidateConfigResponse
		r.ValidateConfig(context.Background(), req, &resp)
		return resp
	}

	nullSize := tftypes.NewValue(tftypes.Number, nil)
	if resp := run("", nullSize); !resp.Diagnostics.HasError() {
		t.Error("neither plan nor size_gb set: want error, got none")
	}
	if resp := run("obj-100", tftypes.NewValue(tftypes.Number, 100)); !resp.Diagnostics.HasError() {
		t.Error("both plan and size_gb set: want error, got none")
	}
	if resp := run("obj-100", nullSize); resp.Diagnostics.HasError() {
		t.Errorf("plan only: unexpected error: %v", resp.Diagnostics)
	}
	// An unknown value resolves at apply time, so validation must not fail.
	if resp := run("", tftypes.NewValue(tftypes.Number, tftypes.UnknownValue)); resp.Diagnostics.HasError() {
		t.Errorf("unknown size_gb: unexpected error: %v", resp.Diagnostics)
	}
}
