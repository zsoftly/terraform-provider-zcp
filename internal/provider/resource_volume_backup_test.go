package provider_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/backup"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeVolumeBackupService satisfies volumeBackupServiceIface.
type fakeVolumeBackupService struct {
	backups   []backup.Backup
	created   *backup.Backup
	createReq backup.CreateRequest
	err       error
	deleted   []string
	// listErrs is consumed one entry per List call (nil entries return the
	// normal fixture data); once exhausted, List falls back to its usual
	// behavior. Used to simulate a transient failure on an early poll.
	listErrs []error
}

func (f *fakeVolumeBackupService) List(_ context.Context, _, _ string) ([]backup.Backup, error) {
	if len(f.listErrs) > 0 {
		e := f.listErrs[0]
		f.listErrs = f.listErrs[1:]
		if e != nil {
			return nil, e
		}
	}
	return f.backups, f.err
}
func (f *fakeVolumeBackupService) Create(_ context.Context, _ string, req backup.CreateRequest) (*backup.Backup, error) {
	f.createReq = req
	return f.created, f.err
}
func (f *fakeVolumeBackupService) Delete(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	if f.err == nil {
		f.backups = nil
	}
	return f.err
}

func volumeBackupConfig(id string) map[string]tftypes.Value {
	cfg := map[string]tftypes.Value{
		"volume":         strVal("root-1234"),
		"interval":       strVal("dailyAt"),
		"billing_cycle":  strVal("hourly"),
		"cloud_provider": strVal("zsoftly"),
		"region":         strVal("yow-1"),
	}
	if id != "" {
		cfg["id"] = strVal(id)
	}
	return cfg
}

func TestVolumeBackupResource_createHappyPath(t *testing.T) {
	svc := &fakeVolumeBackupService{
		created: &backup.Backup{Slug: "root-backup-b1", Interval: "dailyAt", At: 1},
	}
	r := internalprovider.NewVolumeBackupResourceWithService(svc)
	resp := runCreate(t, r, volumeBackupConfig(""))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := stateID(t, resp.State); got != "root-backup-b1" {
		t.Errorf("ID = %q, want root-backup-b1", got)
	}
	// Unset `at` defaults to 1, matching the CLI.
	if svc.createReq.At != 1 {
		t.Errorf("At = %d, want 1 (default)", svc.createReq.At)
	}
	if svc.createReq.PseudoService != "Virtual Machine Backup" {
		t.Errorf("PseudoService = %q, want default", svc.createReq.PseudoService)
	}
}

func TestVolumeBackupResource_deleteHappyPath(t *testing.T) {
	svc := &fakeVolumeBackupService{
		backups: []backup.Backup{{Slug: "root-backup-b1"}},
	}
	r := internalprovider.NewVolumeBackupResourceWithService(svc)
	resp := runDelete(t, r, volumeBackupConfig("root-backup-b1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "root-backup-b1" {
		t.Errorf("Delete called with %v, want [root-backup-b1]", svc.deleted)
	}
}

// TestVolumeBackupResource_deleteAlreadyGoneIsSuccess verifies that a 403
// resource-not-found style error from Delete is treated as already deleted,
// consistent with zcp_vm_backup.
func TestVolumeBackupResource_deleteAlreadyGoneIsSuccess(t *testing.T) {
	svc := &fakeVolumeBackupService{
		err: &apierrors.APIError{StatusCode: 403, Message: "The selected service not found."},
	}
	r := internalprovider.NewVolumeBackupResourceWithService(svc)
	resp := runDelete(t, r, volumeBackupConfig("root-backup-b1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "root-backup-b1" {
		t.Errorf("Delete called with %v, want [root-backup-b1]", svc.deleted)
	}
}

// TestVolumeBackupResource_deleteAlreadyGoneThroughWrappedError verifies the
// not-found detection works through the SDK's real error wrapping: the
// cancellation error comes back wrapped twice (once by the service-cancel
// helper, once by the volume backup delete), so apierrors.IsResourceNotFound's
// errors.As must see through both wraps to recognize the 403 "selected
// service not found" as already deleted.
func TestVolumeBackupResource_deleteAlreadyGoneThroughWrappedError(t *testing.T) {
	inner := &apierrors.APIError{StatusCode: 403, Message: "The selected service not found."}
	wrapped := fmt.Errorf("deleting volume backup %s: %w", "root-backup-b1",
		fmt.Errorf("cancelling service %s: %w", "svc-1", inner))
	svc := &fakeVolumeBackupService{err: wrapped}
	r := internalprovider.NewVolumeBackupResourceWithService(svc)
	resp := runDelete(t, r, volumeBackupConfig("root-backup-b1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "root-backup-b1" {
		t.Errorf("Delete called with %v, want [root-backup-b1]", svc.deleted)
	}
}

// TestVolumeBackupResource_deleteTransientListErrorRetries verifies that a
// single transient (non-not-found) error from the post-delete List call does
// not abort the destroy: the poll keeps going and succeeds once a later List
// call reports the schedule gone.
func TestVolumeBackupResource_deleteTransientListErrorRetries(t *testing.T) {
	svc := &fakeVolumeBackupService{
		backups: []backup.Backup{{Slug: "root-backup-b1"}},
		listErrs: []error{
			&apierrors.APIError{StatusCode: 500, Message: "internal error"},
		},
	}
	r := internalprovider.NewVolumeBackupResourceWithService(svc)
	resp := runDelete(t, r, volumeBackupConfig("root-backup-b1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("a single transient list error should not abort the destroy: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "root-backup-b1" {
		t.Errorf("Delete called with %v, want [root-backup-b1]", svc.deleted)
	}
}

// TestVolumeBackupResource_intervalValidatorRejectsDaily verifies the schema
// validator rejects the legacy "daily" value and names the accepted values.
func TestVolumeBackupResource_intervalValidatorRejectsDaily(t *testing.T) {
	r := internalprovider.NewVolumeBackupResourceWithService(&fakeVolumeBackupService{})
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	attr, ok := schResp.Schema.Attributes["interval"].(rschema.StringAttribute)
	if !ok {
		t.Fatalf("interval attribute is %T, want rschema.StringAttribute", schResp.Schema.Attributes["interval"])
	}
	req := validator.StringRequest{ConfigValue: types.StringValue("daily"), Path: path.Root("interval")}
	var resp validator.StringResponse
	for _, v := range attr.Validators {
		v.ValidateString(context.Background(), req, &resp)
	}
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected \"daily\" to be rejected")
	}
	msg := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(msg, "dailyAt") || !strings.Contains(msg, "hourlyAt") {
		t.Errorf("error detail = %q, want it to name dailyAt and hourlyAt", msg)
	}
}
