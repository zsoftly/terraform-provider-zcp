package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/instance"
	"github.com/zsoftly/zcp-cli/pkg/api/volume"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeVolumeLister satisfies volumeLister.
type fakeVolumeLister struct {
	volumes []volume.Volume
	err     error
}

func (f *fakeVolumeLister) List(_ context.Context, _, _ string) ([]volume.Volume, error) {
	return f.volumes, f.err
}

// instanceDSStateModel mirrors instanceDataSourceModel for state extraction in tests.
type instanceDSStateModel struct {
	Slug       types.String `tfsdk:"slug"`
	Region     types.String `tfsdk:"region"`
	Project    types.String `tfsdk:"project"`
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	State      types.String `tfsdk:"state"`
	PrivateIP  types.String `tfsdk:"private_ip"`
	PublicIP   types.String `tfsdk:"public_ip"`
	RootVolume types.String `tfsdk:"root_volume"`
	Volumes    types.List   `tfsdk:"volumes"`
}

// AC: a VM with a ROOT volume and a data volume attached reports root_volume
// and lists both volumes, root first, ignoring volumes attached to other VMs.
func TestInstanceDataSource_volumes(t *testing.T) {
	getter := &fakeInstanceGetter{
		vm: &instance.VirtualMachine{ID: "vm-uuid-1", Slug: "vm1-abc", Name: "vm1", State: "Running"},
	}
	volLister := &fakeVolumeLister{
		volumes: []volume.Volume{
			{Slug: "data-9999", VirtualMachineID: "vm-uuid-1", VolumeType: ""},
			{Slug: "root-4153", VirtualMachineID: "vm-uuid-1", VolumeType: "ROOT"},
			{Slug: "other-vm-root", VirtualMachineID: "vm-uuid-2", VolumeType: "ROOT"},
		},
	}
	ds := internalprovider.NewInstanceDataSourceWithServices(getter, volLister)
	resp := readDS(t, ds, map[string]tftypes.Value{
		"slug": strVal("vm1-abc"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}

	var got instanceDSStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.RootVolume.ValueString() != "root-4153" {
		t.Errorf("RootVolume = %q, want %q", got.RootVolume.ValueString(), "root-4153")
	}
	var volumeSlugs []string
	if diags := got.Volumes.ElementsAs(context.Background(), &volumeSlugs, false); diags.HasError() {
		t.Fatalf("reading volumes: %v", diags)
	}
	want := []string{"root-4153", "data-9999"}
	if len(volumeSlugs) != len(want) {
		t.Fatalf("Volumes = %v, want %v", volumeSlugs, want)
	}
	for i := range want {
		if volumeSlugs[i] != want[i] {
			t.Errorf("Volumes[%d] = %q, want %q", i, volumeSlugs[i], want[i])
		}
	}
}

// AC: a VM with two ROOT volumes attached (unexpected, but the API allows it)
// keeps the first ROOT as root_volume and lists the second ROOT right after
// it, ahead of ordinary data volumes.
func TestInstanceDataSource_secondRootVolumeKeptAfterFirst(t *testing.T) {
	getter := &fakeInstanceGetter{
		vm: &instance.VirtualMachine{ID: "vm-uuid-1", Slug: "vm1-abc", Name: "vm1", State: "Running"},
	}
	volLister := &fakeVolumeLister{
		volumes: []volume.Volume{
			{Slug: "data-9999", VirtualMachineID: "vm-uuid-1", VolumeType: ""},
			{Slug: "root-4153", VirtualMachineID: "vm-uuid-1", VolumeType: "ROOT"},
			{Slug: "root-4154", VirtualMachineID: "vm-uuid-1", VolumeType: "ROOT"},
		},
	}
	ds := internalprovider.NewInstanceDataSourceWithServices(getter, volLister)
	resp := readDS(t, ds, map[string]tftypes.Value{
		"slug": strVal("vm1-abc"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}

	var got instanceDSStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.RootVolume.ValueString() != "root-4153" {
		t.Errorf("RootVolume = %q, want %q", got.RootVolume.ValueString(), "root-4153")
	}
	var volumeSlugs []string
	if diags := got.Volumes.ElementsAs(context.Background(), &volumeSlugs, false); diags.HasError() {
		t.Fatalf("reading volumes: %v", diags)
	}
	want := []string{"root-4153", "root-4154", "data-9999"}
	if len(volumeSlugs) != len(want) {
		t.Fatalf("Volumes = %v, want %v", volumeSlugs, want)
	}
	for i := range want {
		if volumeSlugs[i] != want[i] {
			t.Errorf("Volumes[%d] = %q, want %q", i, volumeSlugs[i], want[i])
		}
	}
}

// AC: no volume lister configured (data source wired with only a getter)
// leaves root_volume empty and does not error.
func TestInstanceDataSource_noRootVolumeFound(t *testing.T) {
	getter := &fakeInstanceGetter{
		vm: &instance.VirtualMachine{ID: "vm-uuid-1", Slug: "vm1-abc", Name: "vm1", State: "Running"},
	}
	resp := readDS(t, internalprovider.NewInstanceDataSourceWithGetter(getter), map[string]tftypes.Value{
		"slug": strVal("vm1-abc"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got instanceDSStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.RootVolume.ValueString() != "" {
		t.Errorf("RootVolume = %q, want empty", got.RootVolume.ValueString())
	}
}

// AC: a volume-listing failure (e.g. a token without block-storage read
// permission) does not fail the read: it surfaces as a warning naming the
// error, and root_volume/volumes come back empty rather than blocking a read
// of the instance fields that were already resolved.
func TestInstanceDataSource_volumeListError(t *testing.T) {
	getter := &fakeInstanceGetter{
		vm: &instance.VirtualMachine{ID: "vm-uuid-1", Slug: "vm1-abc", Name: "vm1", State: "Running"},
	}
	volLister := &fakeVolumeLister{err: errors.New("API unavailable")}
	resp := readDS(t, internalprovider.NewInstanceDataSourceWithServices(getter, volLister), map[string]tftypes.Value{
		"slug": strVal("vm1-abc"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("a volume-listing failure must not fail the read: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() == 0 {
		t.Fatal("expected a warning on volume list failure, got none")
	}
	if got := resp.Diagnostics.Warnings()[0].Summary(); got != "Failed to list volumes" {
		t.Errorf("unexpected summary: %q", got)
	}
	var got instanceDSStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.RootVolume.ValueString() != "" {
		t.Errorf("RootVolume = %q, want empty", got.RootVolume.ValueString())
	}
	var volumeSlugs []string
	if diags := got.Volumes.ElementsAs(context.Background(), &volumeSlugs, false); diags.HasError() {
		t.Fatalf("reading volumes: %v", diags)
	}
	if len(volumeSlugs) != 0 {
		t.Errorf("Volumes = %v, want empty", volumeSlugs)
	}
}

// AC: the VM's top-level public_ip is null, so public_ip comes from the
// ipaddresses entry marked as the public IP.
func TestInstanceDataSource_publicIPFromIPAddresses(t *testing.T) {
	getter := &fakeInstanceGetter{
		vm: &instance.VirtualMachine{
			ID:    "vm-uuid-1",
			Slug:  "vm1-abc",
			Name:  "vm1",
			State: "Running",
			IPAddresses: []instance.IPAddresses{
				{IPAddress: "203.0.113.10", Type: "IPv4", IPType: "Public IP"},
			},
			PublicIP: nil,
		},
	}
	resp := readDS(t, internalprovider.NewInstanceDataSourceWithGetter(getter), map[string]tftypes.Value{
		"slug": strVal("vm1-abc"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got instanceDSStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.PublicIP.ValueString() != "203.0.113.10" {
		t.Errorf("PublicIP = %q, want %q", got.PublicIP.ValueString(), "203.0.113.10")
	}
}
