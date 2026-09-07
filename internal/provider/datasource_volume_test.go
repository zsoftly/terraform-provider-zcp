package provider_test

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/volume"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// volumeDSStateModel mirrors volumeDataSourceModel for state extraction in tests.
type volumeDSStateModel struct {
	Slug       types.String `tfsdk:"slug"`
	Region     types.String `tfsdk:"region"`
	Project    types.String `tfsdk:"project"`
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Size       types.Int64  `tfsdk:"size"`
	VolumeType types.String `tfsdk:"volume_type"`
	InstanceID types.String `tfsdk:"instance_id"`
	CreatedAt  types.String `tfsdk:"created_at"`
}

// AC: slug matches -> no error, volume fields written to state.
func TestVolumeDataSource_found(t *testing.T) {
	lister := &fakeVolumeLister{
		volumes: []volume.Volume{
			{
				Slug:             "root-4153",
				Name:             "ROOT-4153",
				Size:             "50",
				VolumeType:       "ROOT",
				VirtualMachineID: "vm-uuid-1",
				CreatedAt:        "2026-01-01T00:00:00Z",
			},
		},
	}
	resp := readDS(t, internalprovider.NewVolumeDataSourceWithLister(lister), map[string]tftypes.Value{
		"slug": strVal("root-4153"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}

	var got volumeDSStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "root-4153" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "root-4153")
	}
	if got.Size.ValueInt64() != 50 {
		t.Errorf("Size = %d, want %d", got.Size.ValueInt64(), 50)
	}
	if got.VolumeType.ValueString() != "ROOT" {
		t.Errorf("VolumeType = %q, want %q", got.VolumeType.ValueString(), "ROOT")
	}
	if got.InstanceID.ValueString() != "vm-uuid-1" {
		t.Errorf("InstanceID = %q, want %q", got.InstanceID.ValueString(), "vm-uuid-1")
	}
	if got.CreatedAt.ValueString() != "2026-01-01T00:00:00Z" {
		t.Errorf("CreatedAt = %q, want %q", got.CreatedAt.ValueString(), "2026-01-01T00:00:00Z")
	}
}

// AC: a matched volume whose size is not a valid integer surfaces a
// diagnostic naming the volume and the raw value, instead of silently writing
// 0 to state.
func TestVolumeDataSource_invalidSize(t *testing.T) {
	lister := &fakeVolumeLister{
		volumes: []volume.Volume{
			{Slug: "root-4153", Name: "ROOT-4153", Size: "not-a-number", VolumeType: "ROOT"},
		},
	}
	resp := readDS(t, internalprovider.NewVolumeDataSourceWithLister(lister), map[string]tftypes.Value{
		"slug": strVal("root-4153"),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a non-integer size, got none")
	}
	found := false
	for _, d := range resp.Diagnostics {
		if d.Summary() == "Invalid volume size" &&
			strings.Contains(d.Detail(), "root-4153") &&
			strings.Contains(d.Detail(), "not-a-number") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a diagnostic naming the volume and the raw size, got: %v", resp.Diagnostics)
	}
}

// AC: non-existent slug -> diagnostic error, no panic.
func TestVolumeDataSource_notFound(t *testing.T) {
	lister := &fakeVolumeLister{volumes: []volume.Volume{}}
	resp := readDS(t, internalprovider.NewVolumeDataSourceWithLister(lister), map[string]tftypes.Value{
		"slug": strVal("missing"),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for missing slug, got none")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Volume not found" {
		t.Errorf("unexpected summary: %q", got)
	}
}
