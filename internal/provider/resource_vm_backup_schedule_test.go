package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/instance"
	"github.com/zsoftly/zcp-cli/pkg/api/scheduler"
	"github.com/zsoftly/zcp-cli/pkg/httpclient"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

type fakeVMBackupScheduleService struct {
	policy                   scheduler.Policy
	created                  scheduler.CreateRequest
	updated                  scheduler.UpdateRequest
	paused, resumed, deleted bool
	pauseErr                 error
}
type fakeVMBackupScheduleInstances struct {
	vms []instance.VirtualMachine
	err error
}

func (f *fakeVMBackupScheduleInstances) List(_ context.Context, _, _ string) ([]instance.VirtualMachine, error) {
	return f.vms, f.err
}

func (f *fakeVMBackupScheduleService) Get(_ context.Context, _ string) (*scheduler.Policy, error) {
	return &f.policy, nil
}
func (f *fakeVMBackupScheduleService) Create(_ context.Context, req scheduler.CreateRequest) (*scheduler.Policy, error) {
	f.created = req
	return &f.policy, nil
}
func (f *fakeVMBackupScheduleService) Update(_ context.Context, _ string, req scheduler.UpdateRequest) (*scheduler.Policy, error) {
	f.updated = req
	return &f.policy, nil
}
func (f *fakeVMBackupScheduleService) Delete(_ context.Context, _ string) (*scheduler.ActionResponse, error) {
	f.deleted = true
	return &scheduler.ActionResponse{Status: "Success"}, nil
}
func (f *fakeVMBackupScheduleService) Pause(_ context.Context, _ string) (*scheduler.Policy, error) {
	if f.pauseErr != nil {
		return nil, f.pauseErr
	}
	f.paused = true
	p := f.policy
	p.Status = "paused"
	return &p, nil
}

func TestVMBackupScheduleResourceCreatePreservesStateWhenPauseFails(t *testing.T) {
	day := 2
	svc := &fakeVMBackupScheduleService{pauseErr: errors.New("pause unavailable"), policy: scheduler.Policy{ID: "policy-1", Action: "Virtual Machine Backup", Actionable: &scheduler.VMRef{Slug: "vm-1"}, Region: &scheduler.ScopeRef{Slug: "yow-1"}, Interval: "weeklyOn", Day: &day, At: "02:00", Timezone: "America/Toronto", RetentionPolicy: 3, Status: "active"}}
	resp := runCreate(t, internalprovider.NewVMBackupScheduleResourceWithService(svc, &fakeVMBackupScheduleInstances{vms: []instance.VirtualMachine{{Slug: "vm-1"}}}), vmBackupScheduleConfig("", true))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected pause error")
	}
	if got := stateID(t, resp.State); got != "policy-1" {
		t.Errorf("id=%q, want policy-1", got)
	}
}
func (f *fakeVMBackupScheduleService) Resume(_ context.Context, _ string) (*scheduler.Policy, error) {
	f.resumed = true
	p := f.policy
	p.Status = "active"
	return &p, nil
}

func TestVMBackupScheduleResourceSchemaAndValidation(t *testing.T) {
	r := internalprovider.NewVMBackupScheduleResourceWithService(&fakeVMBackupScheduleService{})
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	for _, name := range []string{"virtual_machine", "interval", "day", "at", "timezone", "retention", "immediate", "paused", "region", "project"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("missing %s attribute", name)
		}
	}
	interval := resp.Schema.Attributes["interval"].(schema.StringAttribute)
	if !interval.Required {
		t.Error("interval must be required")
	}
}

func TestVMBackupScheduleResourceUnconfiguredComputedFieldsKeepState(t *testing.T) {
	r := internalprovider.NewVMBackupScheduleResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	for _, name := range []string{"name", "description"} {
		attribute := schResp.Schema.Attributes[name].(schema.StringAttribute)
		req := planmodifier.StringRequest{Path: path.Root(name), ConfigValue: types.StringNull(), StateValue: types.StringValue("saved"), PlanValue: types.StringUnknown()}
		resp := planmodifier.StringResponse{PlanValue: req.PlanValue}
		for _, modifier := range attribute.PlanModifiers {
			modifier.PlanModifyString(context.Background(), req, &resp)
		}
		if resp.RequiresReplace {
			t.Errorf("%s requires replacement when omitted", name)
		}
	}
}

func TestVMBackupScheduleResourceConfiguredComputedFieldsRequireReplace(t *testing.T) {
	r := internalprovider.NewVMBackupScheduleResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	for _, name := range []string{"name", "description"} {
		attribute := schResp.Schema.Attributes[name].(schema.StringAttribute)
		_, stateRaw := rawFor(t, r, map[string]tftypes.Value{name: strVal("saved")})
		_, planRaw := rawFor(t, r, map[string]tftypes.Value{name: strVal("changed")})
		req := planmodifier.StringRequest{Path: path.Root(name), State: tfsdk.State{Schema: schResp.Schema, Raw: stateRaw}, Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: planRaw}, ConfigValue: types.StringValue("changed"), StateValue: types.StringValue("saved"), PlanValue: types.StringValue("changed")}
		resp := planmodifier.StringResponse{PlanValue: req.PlanValue}
		for _, modifier := range attribute.PlanModifiers {
			modifier.PlanModifyString(context.Background(), req, &resp)
		}
		if !resp.RequiresReplace {
			t.Errorf("%s does not require replacement when configured", name)
		}
	}
}

func TestVMBackupScheduleResourceValidateConfigDoesNotRequireProvider(t *testing.T) {
	r := internalprovider.NewVMBackupScheduleResource().(resource.ResourceWithValidateConfig)
	schResp, raw := rawFor(t, r, vmBackupScheduleConfig("", false))
	var resp resource.ValidateConfigResponse
	r.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schResp.Schema, Raw: raw}}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("validate diagnostics: %v", resp.Diagnostics)
	}
}

func vmBackupScheduleConfig(id string, paused bool) map[string]tftypes.Value {
	cfg := map[string]tftypes.Value{
		"virtual_machine": strVal("vm-1"), "interval": strVal("weeklyOn"),
		"day": intVal(2), "at": strVal("02:00"), "timezone": strVal("America/Toronto"),
		"retention": intVal(3), "region": strVal("yow-1"), "paused": tftypes.NewValue(tftypes.Bool, paused),
	}
	if id != "" {
		cfg["id"] = strVal(id)
	}
	return cfg
}

func TestVMBackupScheduleResourceCRUDPayload(t *testing.T) {
	day := 2
	svc := &fakeVMBackupScheduleService{policy: scheduler.Policy{ID: "policy-1", Slug: "vm-1", Action: "Virtual Machine Backup", Actionable: &scheduler.VMRef{Slug: "vm-1"}, Region: &scheduler.ScopeRef{Slug: "yow-1"}, Interval: "weeklyOn", Day: &day, At: "02:00", Timezone: "America/Toronto", RetentionPolicy: 3, Status: "active"}}
	r := internalprovider.NewVMBackupScheduleResourceWithService(svc, &fakeVMBackupScheduleInstances{vms: []instance.VirtualMachine{{Slug: "vm-1"}}})
	create := runCreate(t, r, vmBackupScheduleConfig("", false))
	if create.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", create.Diagnostics)
	}
	if got := svc.created.Service; got != "Virtual Machine" {
		t.Errorf("service = %q", got)
	}
	if svc.created.Action != "VM Backup" || svc.created.Config.TakeOneImmediately != 0 {
		t.Errorf("unexpected create payload: %#v", svc.created)
	}
	update := runUpdate(t, r, vmBackupScheduleConfig("policy-1", true), vmBackupScheduleConfig("policy-1", false))
	if update.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", update.Diagnostics)
	}
	if svc.updated.RetentionPolicy != 3 || !svc.paused {
		t.Errorf("update=%#v paused=%t", svc.updated, svc.paused)
	}
	delete := runDelete(t, r, vmBackupScheduleConfig("policy-1", false))
	if delete.Diagnostics.HasError() || !svc.deleted {
		t.Fatalf("delete diagnostics: %v deleted=%t", delete.Diagnostics, svc.deleted)
	}
	if got := stateID(t, create.State); got != "policy-1" {
		t.Errorf("id = %q", got)
	}
	if svc.policy.Actionable.Slug != "vm-1" || svc.policy.RetentionPolicy != 3 {
		t.Fatal("invalid schedule fixture")
	}
}

func TestVMBackupScheduleResourceCreateDoesNotWriteWhenVMPreflightFails(t *testing.T) {
	day := 2
	policy := scheduler.Policy{ID: "policy-1", Action: "Virtual Machine Backup", Actionable: &scheduler.VMRef{Slug: "vm-1"}, Region: &scheduler.ScopeRef{Slug: "yow-1"}, Interval: "weeklyOn", Day: &day, At: "02:00", Timezone: "America/Toronto", RetentionPolicy: 3}
	for _, instances := range []*fakeVMBackupScheduleInstances{{err: errors.New("inventory unavailable")}, {vms: nil}} {
		svc := &fakeVMBackupScheduleService{policy: policy}
		resp := runCreate(t, internalprovider.NewVMBackupScheduleResourceWithService(svc, instances), vmBackupScheduleConfig("", false))
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected preflight diagnostic")
		}
		if svc.created.Action != "" {
			t.Fatalf("create request made: %#v", svc.created)
		}
	}
}

func TestVMBackupScheduleResourceOrphanReadAndDeletePreserveConfiguredVM(t *testing.T) {
	day := 2
	svc := &fakeVMBackupScheduleService{policy: scheduler.Policy{ID: "policy-1", Action: "Virtual Machine Backup", Region: &scheduler.ScopeRef{Slug: "yow-1"}, Interval: "weeklyOn", Day: &day, At: "02:00", Timezone: "America/Toronto", RetentionPolicy: 3, Status: "active"}}
	r := internalprovider.NewVMBackupScheduleResourceWithService(svc)
	read := runRead(t, r, vmBackupScheduleConfig("policy-1", false))
	if read.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", read.Diagnostics)
	}
	var virtualMachine string
	if diags := read.State.GetAttribute(context.Background(), path.Root("virtual_machine"), &virtualMachine); diags.HasError() {
		t.Fatalf("virtual_machine: %v", diags)
	}
	if virtualMachine != "vm-1" {
		t.Errorf("virtual_machine = %q, want vm-1", virtualMachine)
	}
	deleted := runDelete(t, r, vmBackupScheduleConfig("policy-1", false))
	if deleted.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", deleted.Diagnostics)
	}
	if !svc.deleted {
		t.Fatal("delete was not called for orphaned policy")
	}
}

func TestVMBackupScheduleResourceImportScopesPolicy(t *testing.T) {
	r := internalprovider.NewVMBackupScheduleResource().(resource.ResourceWithImportState)
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	_, nullState := rawFor(t, r, map[string]tftypes.Value{})
	resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: nullState}}
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: "policy-1/yow-1/project-1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}
	var id, region, project string
	if diags := resp.State.GetAttribute(context.Background(), path.Root("id"), &id); diags.HasError() {
		t.Fatalf("id: %v", diags)
	}
	if diags := resp.State.GetAttribute(context.Background(), path.Root("region"), &region); diags.HasError() {
		t.Fatalf("region: %v", diags)
	}
	if diags := resp.State.GetAttribute(context.Background(), path.Root("project"), &project); diags.HasError() {
		t.Fatalf("project: %v", diags)
	}
	if id != "policy-1" || region != "yow-1" || project != "project-1" {
		t.Errorf("imported %q/%q/%q", id, region, project)
	}
}

func TestVMBackupScheduleSDKCreatePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.URL.Path != "/scheduler-actions" {
			t.Errorf("request = %s %s", req.Method, req.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		var body scheduler.CreateRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
			http.Error(w, "bad JSON", http.StatusBadRequest)
			return
		}
		if body.Service != "Virtual Machine" || body.Action != "VM Backup" || body.Slug != "vm-1" || body.Config.TakeOneImmediately != 1 {
			t.Errorf("payload = %#v", body)
		}
		_, _ = w.Write([]byte(`{"status":"Success","data":{"id":"policy-1","action":"VM Backup","actionable":{"slug":"vm-1"},"region":{"slug":"yow-1"}}}`))
	}))
	defer server.Close()
	svc := scheduler.NewService(httpclient.New(httpclient.Options{BaseURL: server.URL, BearerToken: "test", Timeout: 0}))
	immediate := 1
	if _, err := svc.Create(context.Background(), scheduler.CreateRequest{Service: "Virtual Machine", Slug: "vm-1", Action: "VM Backup", TakeOneImmediately: immediate, Config: scheduler.Config{TakeOneImmediately: immediate}}); err != nil {
		t.Fatalf("create: %v", err)
	}
}
