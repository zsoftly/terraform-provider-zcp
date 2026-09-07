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
	"github.com/zsoftly/zcp-cli/pkg/api/vmbackup"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeVMBackupService satisfies vmBackupServiceIface.
type fakeVMBackupService struct {
	backups      []vmbackup.VMBackup
	afterBackups []vmbackup.VMBackup // returned once Create has been called
	created      bool
	createReq    vmbackup.CreateRequest
	err          error
	deleted      []string
	// listErrs is consumed one entry per List call (nil entries return the
	// normal fixture data); once exhausted, List falls back to its usual
	// behavior. Used to simulate a transient failure on an early poll.
	listErrs []error
}

func (f *fakeVMBackupService) List(_ context.Context, _, _ string) ([]vmbackup.VMBackup, error) {
	if len(f.listErrs) > 0 {
		e := f.listErrs[0]
		f.listErrs = f.listErrs[1:]
		if e != nil {
			return nil, e
		}
	}
	if f.created && f.afterBackups != nil {
		return f.afterBackups, f.err
	}
	return f.backups, f.err
}
func (f *fakeVMBackupService) Create(_ context.Context, _ string, req vmbackup.CreateRequest) (*vmbackup.ActionResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.created = true
	f.createReq = req
	return &vmbackup.ActionResponse{}, nil
}
func (f *fakeVMBackupService) Delete(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	if f.err == nil {
		f.backups = nil
		f.afterBackups = nil
	}
	return f.err
}

func vmBackupConfig(id string) map[string]tftypes.Value {
	cfg := map[string]tftypes.Value{
		"virtual_machine": strVal("vm1-abc"),
		"interval":        strVal("dailyAt"),
		"plan":            strVal("backup-yow"),
		"billing_cycle":   strVal("hourly"),
		"cloud_provider":  strVal("zsoftly"),
		"region":          strVal("yow-1"),
	}
	if id != "" {
		cfg["id"] = strVal(id)
	}
	return cfg
}

func TestVMBackupResource_createResolvesNewSlug(t *testing.T) {
	svc := &fakeVMBackupService{
		backups: []vmbackup.VMBackup{{Slug: "other-backup"}},
		afterBackups: []vmbackup.VMBackup{
			{Slug: "other-backup"},
			{Slug: "vm1-backup-b1", VirtualMachine: &vmbackup.VMRef{Slug: "vm1-abc"}},
		},
	}
	r := internalprovider.NewVMBackupResourceWithService(svc)
	resp := runCreate(t, r, vmBackupConfig(""))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := stateID(t, resp.State); got != "vm1-backup-b1" {
		t.Errorf("ID = %q, want vm1-backup-b1", got)
	}
	if svc.createReq.PseudoService != "vm-backup" {
		t.Errorf("PseudoService = %q, want vm-backup (default)", svc.createReq.PseudoService)
	}
}

// TestVMBackupResource_createBindsByVMSlugAmongMultiple verifies that when
// several new schedules appear at once (e.g. a concurrent apply on another
// VM), the resource binds to the one whose VMSlug() matches the configured
// virtual_machine instead of refusing on ambiguity.
func TestVMBackupResource_createBindsByVMSlugAmongMultiple(t *testing.T) {
	svc := &fakeVMBackupService{
		afterBackups: []vmbackup.VMBackup{
			{Slug: "other-vm-backup", VirtualMachine: &vmbackup.VMRef{Slug: "vm2-xyz"}},
			{Slug: "vm1-backup-b1", VirtualMachine: &vmbackup.VMRef{Slug: "vm1-abc"}},
		},
	}
	r := internalprovider.NewVMBackupResourceWithService(svc)
	resp := runCreate(t, r, vmBackupConfig(""))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := stateID(t, resp.State); got != "vm1-backup-b1" {
		t.Errorf("ID = %q, want vm1-backup-b1 (bound by VM slug, not treated as ambiguous)", got)
	}
}

// TestVMBackupResource_createAmbiguousWhenSameVM verifies that several new
// schedules for the *same* VM still refuse to bind arbitrarily.
func TestVMBackupResource_createAmbiguousWhenSameVM(t *testing.T) {
	svc := &fakeVMBackupService{
		afterBackups: []vmbackup.VMBackup{
			{Slug: "vm1-backup-b1", VirtualMachine: &vmbackup.VMRef{Slug: "vm1-abc"}},
			{Slug: "vm1-backup-b2", VirtualMachine: &vmbackup.VMRef{Slug: "vm1-abc"}},
		},
	}
	r := internalprovider.NewVMBackupResourceWithService(svc)
	resp := runCreate(t, r, vmBackupConfig(""))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when several new backups match the same VM")
	}
}

func TestVMBackupResource_deleteHappyPath(t *testing.T) {
	svc := &fakeVMBackupService{
		backups: []vmbackup.VMBackup{{Slug: "vm1-backup-b1"}},
	}
	r := internalprovider.NewVMBackupResourceWithService(svc)
	resp := runDelete(t, r, vmBackupConfig("vm1-backup-b1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "vm1-backup-b1" {
		t.Errorf("Delete called with %v, want [vm1-backup-b1]", svc.deleted)
	}
}

// TestVMBackupResource_deleteAlreadyGoneIsSuccess verifies that a 403 "The
// selected service not found." from the cancellation endpoint (a slug that no
// longer exists) is treated as a successful delete rather than an error.
func TestVMBackupResource_deleteAlreadyGoneIsSuccess(t *testing.T) {
	svc := &fakeVMBackupService{
		err: &apierrors.APIError{StatusCode: 403, Message: "The selected service not found."},
	}
	r := internalprovider.NewVMBackupResourceWithService(svc)
	resp := runDelete(t, r, vmBackupConfig("vm1-backup-b1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "vm1-backup-b1" {
		t.Errorf("Delete called with %v, want [vm1-backup-b1]", svc.deleted)
	}
}

// TestVMBackupResource_deleteAlreadyGoneThroughWrappedError verifies the
// not-found detection works through the SDK's real error wrapping: the
// cancellation error comes back wrapped twice (once by the service-cancel
// helper, once by the VM backup delete), so apierrors.IsResourceNotFound's
// errors.As must see through both wraps to recognize the 403 "selected
// service not found" as already deleted.
func TestVMBackupResource_deleteAlreadyGoneThroughWrappedError(t *testing.T) {
	inner := &apierrors.APIError{StatusCode: 403, Message: "The selected service not found."}
	wrapped := fmt.Errorf("deleting VM backup %s: %w", "vm1-backup-b1",
		fmt.Errorf("cancelling service %s: %w", "svc-1", inner))
	svc := &fakeVMBackupService{err: wrapped}
	r := internalprovider.NewVMBackupResourceWithService(svc)
	resp := runDelete(t, r, vmBackupConfig("vm1-backup-b1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "vm1-backup-b1" {
		t.Errorf("Delete called with %v, want [vm1-backup-b1]", svc.deleted)
	}
}

// TestVMBackupResource_deleteTransientListErrorRetries verifies that a single
// transient (non-not-found) error from the post-delete List call does not
// abort the destroy: the poll keeps going and succeeds once a later List call
// reports the schedule gone.
func TestVMBackupResource_deleteTransientListErrorRetries(t *testing.T) {
	svc := &fakeVMBackupService{
		backups: []vmbackup.VMBackup{{Slug: "vm1-backup-b1"}},
		listErrs: []error{
			&apierrors.APIError{StatusCode: 500, Message: "internal error"},
		},
	}
	r := internalprovider.NewVMBackupResourceWithService(svc)
	resp := runDelete(t, r, vmBackupConfig("vm1-backup-b1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("a single transient list error should not abort the destroy: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "vm1-backup-b1" {
		t.Errorf("Delete called with %v, want [vm1-backup-b1]", svc.deleted)
	}
}

// TestVMBackupResource_intervalValidatorRejectsDaily verifies the schema
// validator rejects the legacy "daily" value and names the accepted values.
func TestVMBackupResource_intervalValidatorRejectsDaily(t *testing.T) {
	r := internalprovider.NewVMBackupResourceWithService(&fakeVMBackupService{})
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
