package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/snapshot"
	"github.com/zsoftly/zcp-cli/pkg/api/vmsnapshot"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// rawFor builds a full raw value for a resource schema, filling unset
// attributes with nulls (mirrors readDS for resources).
func rawFor(t *testing.T, r resource.Resource, config map[string]tftypes.Value) (resource.SchemaResponse, tftypes.Value) {
	t.Helper()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	obj, ok := tfType.(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is %T, want tftypes.Object", tfType)
	}
	vals := map[string]tftypes.Value{}
	for name, attrType := range obj.AttributeTypes {
		if v, set := config[name]; set {
			vals[name] = v
		} else {
			vals[name] = tftypes.NewValue(attrType, nil)
		}
	}
	return schResp, tftypes.NewValue(tfType, vals)
}

func runCreate(t *testing.T, r resource.Resource, config map[string]tftypes.Value) resource.CreateResponse {
	t.Helper()
	schResp, planVal := rawFor(t, r, config)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	createReq := resource.CreateRequest{Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: planVal}}
	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)}}
	r.Create(context.Background(), createReq, createResp)
	return *createResp
}

func runRead(t *testing.T, r resource.Resource, config map[string]tftypes.Value) resource.ReadResponse {
	t.Helper()
	schResp, stateVal := rawFor(t, r, config)
	readReq := resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), readReq, readResp)
	return *readResp
}

func runDelete(t *testing.T, r resource.Resource, config map[string]tftypes.Value) resource.DeleteResponse {
	t.Helper()
	schResp, stateVal := rawFor(t, r, config)
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

func stateID(t *testing.T, raw tfsdk.State) string {
	t.Helper()
	var id *string
	if diags := raw.GetAttribute(context.Background(), path.Root("id"), &id); diags.HasError() {
		t.Fatalf("reading id: %v", diags)
	}
	if id == nil {
		return ""
	}
	return *id
}

// --- zcp_vm_snapshot ---

type fakeVMSnapshotService struct {
	snaps      []vmsnapshot.VMSnapshot
	afterSnaps []vmsnapshot.VMSnapshot // returned once Create has been called
	created    bool
	err        error
	deleted    []string
}

func (f *fakeVMSnapshotService) List(_ context.Context, _, _ string) ([]vmsnapshot.VMSnapshot, error) {
	if f.created && f.afterSnaps != nil {
		return f.afterSnaps, f.err
	}
	return f.snaps, f.err
}
func (f *fakeVMSnapshotService) Create(_ context.Context, _ string, _ vmsnapshot.CreateRequest) (*vmsnapshot.ActionResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.created = true
	return &vmsnapshot.ActionResponse{}, nil
}
func (f *fakeVMSnapshotService) Delete(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	if f.err == nil {
		f.snaps = nil
		f.afterSnaps = nil
	}
	return f.err
}

func vmSnapshotConfig(id string) map[string]tftypes.Value {
	cfg := map[string]tftypes.Value{
		"virtual_machine": strVal("vm1-abc"),
		"name":            strVal("pre-upgrade"),
		"billing_cycle":   strVal("monthly"),
		"cloud_provider":  strVal("zsoftly"),
		"region":          strVal("yow-1"),
	}
	if id != "" {
		cfg["id"] = strVal(id)
	}
	return cfg
}

func TestVMSnapshotResource_createResolvesFromList(t *testing.T) {
	svc := &fakeVMSnapshotService{
		snaps: []vmsnapshot.VMSnapshot{{Slug: "old-snap", Name: "old"}},
		afterSnaps: []vmsnapshot.VMSnapshot{
			{Slug: "old-snap", Name: "old"},
			{Slug: "pre-upgrade-s1", Name: "pre-upgrade", State: "Ready"},
		},
	}
	r := internalprovider.NewVMSnapshotResourceWithService(svc)
	resp := runCreate(t, r, vmSnapshotConfig(""))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := stateID(t, resp.State); got != "pre-upgrade-s1" {
		t.Errorf("ID = %q, want pre-upgrade-s1", got)
	}
}

func TestVMSnapshotResource_createServiceError(t *testing.T) {
	svc := &fakeVMSnapshotService{err: errors.New("vm not running")}
	r := internalprovider.NewVMSnapshotResourceWithService(svc)
	resp := runCreate(t, r, vmSnapshotConfig(""))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func TestVMSnapshotResource_readNotFoundRemoves(t *testing.T) {
	svc := &fakeVMSnapshotService{}
	r := internalprovider.NewVMSnapshotResourceWithService(svc)
	resp := runRead(t, r, vmSnapshotConfig("pre-upgrade-s1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestVMSnapshotResource_deleteHappyPath(t *testing.T) {
	svc := &fakeVMSnapshotService{
		snaps: []vmsnapshot.VMSnapshot{{Slug: "pre-upgrade-s1", Name: "pre-upgrade"}},
	}
	r := internalprovider.NewVMSnapshotResourceWithService(svc)
	resp := runDelete(t, r, vmSnapshotConfig("pre-upgrade-s1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "pre-upgrade-s1" {
		t.Errorf("Delete called with %v, want [pre-upgrade-s1]", svc.deleted)
	}
}

// --- zcp_volume_snapshot ---

type fakeVolumeSnapshotService struct {
	snaps   []snapshot.Snapshot
	created *snapshot.Snapshot
	err     error
	deleted []string
}

func (f *fakeVolumeSnapshotService) List(_ context.Context, _, _ string) ([]snapshot.Snapshot, error) {
	return f.snaps, f.err
}
func (f *fakeVolumeSnapshotService) Create(_ context.Context, _ string, _ snapshot.CreateRequest) (*snapshot.Snapshot, error) {
	return f.created, f.err
}
func (f *fakeVolumeSnapshotService) Delete(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	if f.err == nil {
		f.snaps = nil
	}
	return f.err
}

func volumeSnapshotConfig(id string) map[string]tftypes.Value {
	cfg := map[string]tftypes.Value{
		"volume":         strVal("root-1234"),
		"name":           strVal("nightly"),
		"billing_cycle":  strVal("hourly"),
		"cloud_provider": strVal("zsoftly"),
		"region":         strVal("yow-1"),
	}
	if id != "" {
		cfg["id"] = strVal(id)
	}
	return cfg
}

func TestVolumeSnapshotResource_createHappyPath(t *testing.T) {
	svc := &fakeVolumeSnapshotService{
		created: &snapshot.Snapshot{Slug: "nightly-s1", Name: "nightly"},
	}
	r := internalprovider.NewVolumeSnapshotResourceWithService(svc)
	resp := runCreate(t, r, volumeSnapshotConfig(""))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := stateID(t, resp.State); got != "nightly-s1" {
		t.Errorf("ID = %q, want nightly-s1", got)
	}
}

func TestVolumeSnapshotResource_deleteHappyPath(t *testing.T) {
	svc := &fakeVolumeSnapshotService{
		snaps: []snapshot.Snapshot{{Slug: "nightly-s1", Name: "nightly"}},
	}
	r := internalprovider.NewVolumeSnapshotResourceWithService(svc)
	resp := runDelete(t, r, volumeSnapshotConfig("nightly-s1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "nightly-s1" {
		t.Errorf("Delete called with %v, want [nightly-s1]", svc.deleted)
	}
}
