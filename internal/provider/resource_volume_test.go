package provider_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/volume"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeVolumeService satisfies volumeServiceIface.
type fakeVolumeService struct {
	created     *volume.Volume
	list        []volume.Volume
	createErr   error
	attached    []string // "<vol>->vm"
	detached    []string
	deleted     []string
	createReq   volume.CreateRequest
	deleteErr   error
	attachErr   error
	detachErr   error
	gotCreateOK bool
}

func (f *fakeVolumeService) Create(_ context.Context, req volume.CreateRequest) (*volume.Volume, error) {
	f.createReq = req
	f.gotCreateOK = true
	return f.created, f.createErr
}
func (f *fakeVolumeService) List(_ context.Context, _, _ string) ([]volume.Volume, error) {
	return f.list, nil
}
func (f *fakeVolumeService) Attach(_ context.Context, volumeSlug, vmSlug string) (*volume.Volume, error) {
	f.attached = append(f.attached, volumeSlug+"->"+vmSlug)
	return f.created, f.attachErr
}
func (f *fakeVolumeService) Detach(_ context.Context, volumeSlug string) (*volume.Volume, error) {
	f.detached = append(f.detached, volumeSlug)
	return f.created, f.detachErr
}
func (f *fakeVolumeService) Delete(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	return f.deleteErr
}

type volumeStateModel struct {
	ID              types.String   `tfsdk:"id"`
	Name            types.String   `tfsdk:"name"`
	CloudProvider   types.String   `tfsdk:"cloud_provider"`
	Region          types.String   `tfsdk:"region"`
	BillingCycle    types.String   `tfsdk:"billing_cycle"`
	StorageCategory types.String   `tfsdk:"storage_category"`
	Plan            types.String   `tfsdk:"plan"`
	Size            types.Int64    `tfsdk:"size"`
	Project         types.String   `tfsdk:"project"`
	VM              types.String   `tfsdk:"vm"`
	Slug            types.String   `tfsdk:"slug"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
}

func volumeSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewVolumeResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func volumeTFType(t *testing.T) tftypes.Type {
	t.Helper()
	return volumeSchema(t).Schema.Type().TerraformType(context.Background())
}

// volumeValues builds a full attribute map. plan/size/vm are passed explicitly
// (nil string/int leaves them null).
func volumeValues(t *testing.T, id, plan string, size *int64, vm string) map[string]tftypes.Value {
	t.Helper()
	nullStr := func() tftypes.Value { return tftypes.NewValue(tftypes.String, nil) }
	idVal := nullStr()
	if id != "" {
		idVal = tftypes.NewValue(tftypes.String, id)
	}
	planVal := nullStr()
	if plan != "" {
		planVal = tftypes.NewValue(tftypes.String, plan)
	}
	sizeVal := tftypes.NewValue(tftypes.Number, nil)
	if size != nil {
		sizeVal = tftypes.NewValue(tftypes.Number, *size)
	}
	vmVal := nullStr()
	if vm != "" {
		vmVal = tftypes.NewValue(tftypes.String, vm)
	}
	return map[string]tftypes.Value{
		"id":               idVal,
		"name":             tftypes.NewValue(tftypes.String, "vol1"),
		"cloud_provider":   tftypes.NewValue(tftypes.String, "nimbo"),
		"region":           tftypes.NewValue(tftypes.String, "yow-1"),
		"billing_cycle":    tftypes.NewValue(tftypes.String, "hourly"),
		"storage_category": tftypes.NewValue(tftypes.String, "nvme"),
		"plan":             planVal,
		"size":             sizeVal,
		"project":          nullStr(),
		"vm":               vmVal,
		"slug":             idVal,
		"timeouts":         timeoutsNull(t, volumeSchema(t)),
	}
}

func createVolume(t *testing.T, svc *fakeVolumeService, plan string, size *int64, vm string) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewVolumeResourceWithService(svc)
	schResp := volumeSchema(t)
	tfType := volumeTFType(t)
	planVal := tftypes.NewValue(tfType, volumeValues(t, "", plan, size, vm))
	createReq := resource.CreateRequest{Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: planVal}}
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)},
	}
	r.Create(context.Background(), createReq, createResp)
	return *createResp
}

func TestVolumeResource_createWithPlan(t *testing.T) {
	svc := &fakeVolumeService{created: &volume.Volume{Slug: "vol1-abc", Name: "vol1"}}
	resp := createVolume(t, svc, "b1g1", nil, "")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if svc.createReq.Plan != "b1g1" || svc.createReq.IsCustomPlan {
		t.Errorf("expected plan-based request, got plan=%q custom=%v", svc.createReq.Plan, svc.createReq.IsCustomPlan)
	}
	var got volumeStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "vol1-abc" {
		t.Errorf("ID = %q, want vol1-abc", got.ID.ValueString())
	}
}

func TestVolumeResource_createWithSize(t *testing.T) {
	svc := &fakeVolumeService{created: &volume.Volume{Slug: "vol1-abc", Name: "vol1"}}
	size := int64(50)
	resp := createVolume(t, svc, "", &size, "")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if !svc.createReq.IsCustomPlan || svc.createReq.CustomPlan == nil || svc.createReq.CustomPlan.Storage != 50 {
		t.Errorf("expected custom-plan size 50, got %+v", svc.createReq.CustomPlan)
	}
}

func TestVolumeResource_createAttachesVM(t *testing.T) {
	svc := &fakeVolumeService{created: &volume.Volume{Slug: "vol1-abc", Name: "vol1"}}
	resp := createVolume(t, svc, "b1g1", nil, "vm-xyz")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if svc.createReq.VirtualMachine != "vm-xyz" {
		t.Errorf("VirtualMachine = %q, want vm-xyz", svc.createReq.VirtualMachine)
	}
}

func TestVolumeResource_validateConfigBothSet(t *testing.T) {
	r := internalprovider.NewVolumeResource().(resource.ResourceWithValidateConfig)
	schResp := volumeSchema(t)
	tfType := volumeTFType(t)
	size := int64(50)
	cfgVal := tftypes.NewValue(tfType, volumeValues(t, "", "b1g1", &size, ""))
	req := resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schResp.Schema, Raw: cfgVal}}
	var resp resource.ValidateConfigResponse
	r.ValidateConfig(context.Background(), req, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when both plan and size are set")
	}
}

func TestVolumeResource_validateConfigNeitherSet(t *testing.T) {
	r := internalprovider.NewVolumeResource().(resource.ResourceWithValidateConfig)
	schResp := volumeSchema(t)
	tfType := volumeTFType(t)
	cfgVal := tftypes.NewValue(tfType, volumeValues(t, "", "", nil, ""))
	req := resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schResp.Schema, Raw: cfgVal}}
	var resp resource.ValidateConfigResponse
	r.ValidateConfig(context.Background(), req, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when neither plan nor size is set")
	}
}

func TestVolumeResource_validateConfigPlanOnly(t *testing.T) {
	r := internalprovider.NewVolumeResource().(resource.ResourceWithValidateConfig)
	schResp := volumeSchema(t)
	tfType := volumeTFType(t)
	cfgVal := tftypes.NewValue(tfType, volumeValues(t, "", "b1g1", nil, ""))
	req := resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schResp.Schema, Raw: cfgVal}}
	var resp resource.ValidateConfigResponse
	r.ValidateConfig(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("plan-only config should validate: %v", resp.Diagnostics)
	}
}

func readVolume(t *testing.T, svc *fakeVolumeService, slug string) resource.ReadResponse {
	t.Helper()
	r := internalprovider.NewVolumeResourceWithService(svc)
	schResp := volumeSchema(t)
	tfType := volumeTFType(t)
	stateVal := tftypes.NewValue(tfType, volumeValues(t, slug, "b1g1", nil, ""))
	readReq := resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), readReq, readResp)
	return *readResp
}

func TestVolumeResource_readFound(t *testing.T) {
	svc := &fakeVolumeService{list: []volume.Volume{{Slug: "vol1-abc", Name: "renamed"}}}
	resp := readVolume(t, svc, "vol1-abc")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got volumeStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Name.ValueString() != "renamed" {
		t.Errorf("Name = %q, want renamed", got.Name.ValueString())
	}
}

func TestVolumeResource_readNotFound(t *testing.T) {
	svc := &fakeVolumeService{list: []volume.Volume{}}
	resp := readVolume(t, svc, "missing")
	if resp.Diagnostics.HasError() {
		t.Fatalf("not-found read should not error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected null state after RemoveResource")
	}
}

func deleteVolume(t *testing.T, svc *fakeVolumeService, slug, vm string) resource.DeleteResponse {
	t.Helper()
	r := internalprovider.NewVolumeResourceWithService(svc)
	schResp := volumeSchema(t)
	tfType := volumeTFType(t)
	stateVal := tftypes.NewValue(tfType, volumeValues(t, slug, "b1g1", nil, vm))
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

func TestVolumeResource_deleteDetachesWhenAttached(t *testing.T) {
	svc := &fakeVolumeService{}
	resp := deleteVolume(t, svc, "vol1-abc", "vm-xyz")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.detached) != 1 || svc.detached[0] != "vol1-abc" {
		t.Errorf("Detach called with %v, want [vol1-abc]", svc.detached)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "vol1-abc" {
		t.Errorf("Delete called with %v, want [vol1-abc]", svc.deleted)
	}
}

func TestVolumeResource_deleteUnattachedSkipsDetach(t *testing.T) {
	svc := &fakeVolumeService{}
	resp := deleteVolume(t, svc, "vol1-abc", "")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.detached) != 0 {
		t.Errorf("Detach should not be called for an unattached volume, got %v", svc.detached)
	}
}

func TestVolumeResource_delete404IsNoOp(t *testing.T) {
	svc := &fakeVolumeService{deleteErr: &apierrors.APIError{StatusCode: 404}}
	resp := deleteVolume(t, svc, "gone", "")
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 delete should be a no-op: %v", resp.Diagnostics)
	}
}

func TestVolumeResource_updateAttachDetach(t *testing.T) {
	svc := &fakeVolumeService{created: &volume.Volume{Slug: "vol1-abc"}}
	r := internalprovider.NewVolumeResourceWithService(svc)
	schResp := volumeSchema(t)
	tfType := volumeTFType(t)
	stateVal := tftypes.NewValue(tfType, volumeValues(t, "vol1-abc", "b1g1", nil, "vm-old"))
	planVal := tftypes.NewValue(tfType, volumeValues(t, "vol1-abc", "b1g1", nil, "vm-new"))
	updateReq := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schResp.Schema, Raw: planVal},
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Update(context.Background(), updateReq, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", updateResp.Diagnostics)
	}
	if len(svc.detached) != 1 {
		t.Errorf("expected detach from old VM, got %v", svc.detached)
	}
	if len(svc.attached) != 1 || svc.attached[0] != "vol1-abc->vm-new" {
		t.Errorf("expected attach to vm-new, got %v", svc.attached)
	}
}
