package provider_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/instance"
	"github.com/zsoftly/zcp-cli/pkg/api/ipaddress"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeInstanceService satisfies instanceServiceIface.
type fakeInstanceService struct {
	created         *instance.VirtualMachine
	got             *instance.VirtualMachine
	logs            []instance.ActivityLog
	createReq       instance.CreateRequest
	createErr       error
	getErr          error
	gotQueue        []*instance.VirtualMachine
	metaState       string
	metaErr         error
	deleted         []string
	expunged        []bool
	deletedPublicIP []bool
	deleteErr       error
	getCalls        int

	// update-path capture
	renamedTo     string
	changedPlan   *instance.ChangePlanRequest
	changePlanErr error
	changedUD     *string
	tagsCreated   map[string]string
	tagsDeleted   []string
	startCalled   int
	stopCalled    int

	// staleCache simulates the CMP's lagging cached state: Stop/Start do NOT
	// update got.State. They update metaState (the live view) and append a fresh
	// VM.ACTION=SUCCESS log, so readiness must come from the /meta endpoint.
	staleCache bool
	actionSeq  int
}

func (f *fakeInstanceService) pushAction() {
	f.actionSeq++
	f.logs = append([]instance.ActivityLog{{
		Action: "VM.ACTION", Status: "SUCCESS",
		CreatedAt: fmt.Sprintf("2026-06-22T10:00:%02dZ", f.actionSeq),
	}}, f.logs...)
}

func (f *fakeInstanceService) Create(_ context.Context, req instance.CreateRequest) (*instance.VirtualMachine, error) {
	f.createReq = req
	return f.created, f.createErr
}
func (f *fakeInstanceService) Get(_ context.Context, _ string) (*instance.VirtualMachine, error) {
	f.getCalls++
	if len(f.gotQueue) > 0 {
		vm := f.gotQueue[0]
		f.gotQueue = f.gotQueue[1:]
		return vm, nil
	}
	return f.got, f.getErr
}

// Meta returns the live state: metaState when set, else the cached got.State.
func (f *fakeInstanceService) Meta(_ context.Context, _ string) (*instance.VMMeta, error) {
	if f.metaErr != nil {
		return nil, f.metaErr
	}
	meta := &instance.VMMeta{State: f.metaState}
	if f.got != nil {
		meta.ID = f.got.ID
		meta.Name = f.got.Name
		if meta.State == "" {
			meta.State = f.got.State
		}
	}
	return meta, nil
}
func (f *fakeInstanceService) ActivityLogs(_ context.Context, _ string) ([]instance.ActivityLog, error) {
	return f.logs, nil
}
func (f *fakeInstanceService) ChangeLabel(_ context.Context, _ string, name string) error {
	f.renamedTo = name
	if f.got != nil {
		f.got.Name = name
	}
	return nil
}
func (f *fakeInstanceService) ChangePlan(_ context.Context, _ string, req instance.ChangePlanRequest) (*instance.ActionResponse, error) {
	f.changedPlan = &req
	if f.changePlanErr == nil && f.staleCache {
		f.pushAction()
	}
	return &instance.ActionResponse{}, f.changePlanErr
}
func (f *fakeInstanceService) ChangeStartupScript(_ context.Context, _ string, req instance.ChangeStartupScriptRequest) (*instance.ActionResponse, error) {
	ud := req.UserData
	f.changedUD = &ud
	return &instance.ActionResponse{}, nil
}
func (f *fakeInstanceService) CreateTag(_ context.Context, _ string, req instance.TagRequest) (*instance.ActionResponse, error) {
	if f.tagsCreated == nil {
		f.tagsCreated = map[string]string{}
	}
	f.tagsCreated[req.Key] = req.Value
	return &instance.ActionResponse{}, nil
}
func (f *fakeInstanceService) DeleteTag(_ context.Context, _ string, key string) error {
	f.tagsDeleted = append(f.tagsDeleted, key)
	return nil
}
func (f *fakeInstanceService) Start(_ context.Context, _ string) (*instance.ActionResponse, error) {
	f.startCalled++
	if f.staleCache {
		f.metaState = "Running"
		f.pushAction()
		return &instance.ActionResponse{}, nil
	}
	if f.got != nil {
		f.got.State = "Running"
	}
	return &instance.ActionResponse{}, nil
}
func (f *fakeInstanceService) Stop(_ context.Context, _ string) (*instance.ActionResponse, error) {
	f.stopCalled++
	if f.staleCache {
		f.metaState = "Stopped"
		f.pushAction()
		return &instance.ActionResponse{}, nil
	}
	if f.got != nil {
		f.got.State = "Stopped"
	}
	return &instance.ActionResponse{}, nil
}
func (f *fakeInstanceService) Delete(_ context.Context, slug string, expunge, deletePublicIP bool) error {
	f.deleted = append(f.deleted, slug)
	f.expunged = append(f.expunged, expunge)
	f.deletedPublicIP = append(f.deletedPublicIP, deletePublicIP)
	return f.deleteErr
}

type instanceStateModel struct {
	ID              types.String   `tfsdk:"id"`
	Name            types.String   `tfsdk:"name"`
	CloudProvider   types.String   `tfsdk:"cloud_provider"`
	Region          types.String   `tfsdk:"region"`
	Template        types.String   `tfsdk:"template"`
	Plan            types.String   `tfsdk:"plan"`
	BillingCycle    types.String   `tfsdk:"billing_cycle"`
	Project         types.String   `tfsdk:"project"`
	SSHKey          types.String   `tfsdk:"ssh_key"`
	Network         types.String   `tfsdk:"network"`
	NetworkPlan     types.String   `tfsdk:"network_plan"`
	AssignPublicIP  types.Bool     `tfsdk:"assign_public_ip"`
	StorageCategory types.String   `tfsdk:"storage_category"`
	UserData        types.String   `tfsdk:"user_data"`
	Tags            types.Map      `tfsdk:"tags"`
	Slug            types.String   `tfsdk:"slug"`
	State           types.String   `tfsdk:"state"`
	PrivateIP       types.String   `tfsdk:"private_ip"`
	PublicIP        types.String   `tfsdk:"public_ip"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
}

func instanceSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewInstanceResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func instanceTFType(t *testing.T) tftypes.Type {
	t.Helper()
	return instanceSchema(t).Schema.Type().TerraformType(context.Background())
}

// instanceValues builds a full attribute map with the given id/required inputs;
// computed fields are null.
func instanceValues(t *testing.T, id string) map[string]tftypes.Value {
	t.Helper()
	null := func() tftypes.Value { return tftypes.NewValue(tftypes.String, nil) }
	idVal := null()
	if id != "" {
		idVal = tftypes.NewValue(tftypes.String, id)
	}
	return map[string]tftypes.Value{
		"id":               idVal,
		"name":             tftypes.NewValue(tftypes.String, "vm1"),
		"cloud_provider":   tftypes.NewValue(tftypes.String, "nimbo"),
		"region":           tftypes.NewValue(tftypes.String, "yow-1"),
		"template":         tftypes.NewValue(tftypes.String, "ubuntu-24"),
		"plan":             tftypes.NewValue(tftypes.String, "ci1.small"),
		"billing_cycle":    tftypes.NewValue(tftypes.String, "hourly"),
		"project":          null(),
		"ssh_key":          null(),
		"network":          null(),
		"network_plan":     null(),
		"assign_public_ip": tftypes.NewValue(tftypes.Bool, nil),
		"storage_category": null(),
		"user_data":        null(),
		"tags":             tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, nil),
		"slug":             idVal,
		"state":            null(),
		"private_ip":       null(),
		"public_ip":        null(),
		"timeouts":         timeoutsNull(t, instanceSchema(t)),
	}
}

func createInstance(t *testing.T, svc *fakeInstanceService) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewInstanceResourceWithService(svc)
	schResp := instanceSchema(t)
	tfType := instanceTFType(t)
	planVal := tftypes.NewValue(tfType, instanceValues(t, ""))
	createReq := resource.CreateRequest{Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: planVal}}
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)},
	}
	r.Create(context.Background(), createReq, createResp)
	return *createResp
}

func TestInstanceResource_createWaitsForRunning(t *testing.T) {
	pubIP := "203.0.113.5"
	// Create polls via Get; return a Running VM on the first poll so it settles
	// immediately (no interval sleep) and IPs are populated.
	svc := &fakeInstanceService{
		created: &instance.VirtualMachine{Slug: "vm1-abc", State: "Pending"},
		got: &instance.VirtualMachine{
			Slug:     "vm1-abc",
			State:    "Running",
			PublicIP: &pubIP,
			Networks: []instance.VMNetwork{
				{IsDefault: true, Pivot: &instance.VMNetworkIP{IsDefault: true, IPAddress: "10.0.0.4"}},
			},
		},
	}
	resp := createInstance(t, svc)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got instanceStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.State.ValueString() != "Running" {
		t.Errorf("State = %q, want Running", got.State.ValueString())
	}
	if got.PrivateIP.ValueString() != "10.0.0.4" {
		t.Errorf("PrivateIP = %q, want 10.0.0.4", got.PrivateIP.ValueString())
	}
	if got.PublicIP.ValueString() != "203.0.113.5" {
		t.Errorf("PublicIP = %q, want 203.0.113.5", got.PublicIP.ValueString())
	}
	if got.ID.ValueString() != "vm1-abc" || got.Slug.ValueString() != "vm1-abc" {
		t.Errorf("ID/Slug = %q/%q, want vm1-abc", got.ID.ValueString(), got.Slug.ValueString())
	}
}

// TestInstanceResource_createReadyViaLiveMeta verifies /meta is the readiness
// signal: the cached state is still "Starting" but the live meta view reports
// Running, so create returns and stamps state as Running.
func TestInstanceResource_createReadyViaLiveMeta(t *testing.T) {
	pubIP := "203.0.113.9"
	svc := &fakeInstanceService{
		created: &instance.VirtualMachine{Slug: "vm1-abc", State: "Starting"},
		// Get returns the stale "Starting" state...
		got: &instance.VirtualMachine{
			Slug:     "vm1-abc",
			State:    "Starting",
			PublicIP: &pubIP,
			Networks: []instance.VMNetwork{{IsDefault: true, Pivot: &instance.VMNetworkIP{IsDefault: true, IPAddress: "10.0.0.7"}}},
		},
		// ...but the live /meta view says the VM is up.
		metaState: "Running",
	}
	resp := createInstance(t, svc)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got instanceStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.State.ValueString() != "Running" {
		t.Errorf("State = %q, want Running (meta-confirmed despite stale Starting)", got.State.ValueString())
	}
	if got.PublicIP.ValueString() != "203.0.113.9" || got.PrivateIP.ValueString() != "10.0.0.7" {
		t.Errorf("IPs not populated: public=%q private=%q", got.PublicIP.ValueString(), got.PrivateIP.ValueString())
	}
}

// TestInstanceResource_createFailsViaActivityLog verifies a FAILED VM.CREATE log
// errors out (fast) regardless of the state field.
func TestInstanceResource_createFailsViaActivityLog(t *testing.T) {
	svc := &fakeInstanceService{
		created: &instance.VirtualMachine{Slug: "vm1-abc", State: "Starting"},
		got:     &instance.VirtualMachine{Slug: "vm1-abc", State: "Starting"},
		logs:    []instance.ActivityLog{{Action: "VM.CREATE", Status: "FAILED"}},
	}
	resp := createInstance(t, svc)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when VM.CREATE log is FAILED")
	}
}

func TestInstanceResource_createFailsFastOnTerminalState(t *testing.T) {
	// A terminal provisioning state errors immediately instead of blocking for the
	// full create timeout.
	svc := &fakeInstanceService{
		created: &instance.VirtualMachine{Slug: "vm1-abc", State: "Pending"},
		got:     &instance.VirtualMachine{Slug: "vm1-abc", State: "Error"},
	}
	resp := createInstance(t, svc)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when instance enters a terminal state")
	}
}

func TestInstanceResource_createServiceError(t *testing.T) {
	svc := &fakeInstanceService{createErr: errors.New("quota exceeded")}
	resp := createInstance(t, svc)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure")
	}
}

// TestInstanceResource_createAttachesExistingNetwork verifies that setting
// `network` sends networks:[slug] and NO network_plan (so no untracked network
// is auto-created), and that assign_public_ip=false sets IsPublic=false.
func TestInstanceResource_createAttachesExistingNetwork(t *testing.T) {
	svc := &fakeInstanceService{
		created: &instance.VirtualMachine{Slug: "vm1-abc", State: "Pending"},
		got:     &instance.VirtualMachine{Slug: "vm1-abc", State: "Running"},
	}
	r := internalprovider.NewInstanceResourceWithService(svc)
	schResp := instanceSchema(t)
	tfType := instanceTFType(t)
	vals := instanceValues(t, "")
	vals["network"] = tftypes.NewValue(tftypes.String, "app-net")
	vals["assign_public_ip"] = tftypes.NewValue(tftypes.Bool, false)
	createReq := resource.CreateRequest{Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, vals)}}
	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)}}
	r.Create(context.Background(), createReq, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", createResp.Diagnostics)
	}
	if len(svc.createReq.Networks) != 1 || svc.createReq.Networks[0] != "app-net" {
		t.Errorf("Networks = %v, want [app-net]", svc.createReq.Networks)
	}
	if svc.createReq.NetworkPlan != "" {
		t.Errorf("NetworkPlan = %q, want empty (no auto-create)", svc.createReq.NetworkPlan)
	}
	if svc.createReq.IsPublic {
		t.Error("IsPublic = true, want false (assign_public_ip=false)")
	}
}

// TestInstanceResource_validateNetworkConflict verifies network + network_plan
// together is rejected at plan time.
func TestInstanceResource_validateNetworkConflict(t *testing.T) {
	r := internalprovider.NewInstanceResource().(resource.ResourceWithValidateConfig)
	schResp := instanceSchema(t)
	tfType := instanceTFType(t)
	vals := instanceValues(t, "")
	vals["network"] = tftypes.NewValue(tftypes.String, "app-net")
	vals["network_plan"] = tftypes.NewValue(tftypes.String, "pnet-yow")
	req := resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, vals)}}
	var resp resource.ValidateConfigResponse
	r.ValidateConfig(context.Background(), req, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when both network and network_plan are set")
	}
}

// strVal returns a tftypes string value (null when empty).
func strVal(s string) tftypes.Value {
	if s == "" {
		return tftypes.NewValue(tftypes.String, nil)
	}
	return tftypes.NewValue(tftypes.String, s)
}

// tagsVal builds a tftypes map value (null when m is nil).
func tagsVal(m map[string]string) tftypes.Value {
	mt := tftypes.Map{ElementType: tftypes.String}
	if m == nil {
		return tftypes.NewValue(mt, nil)
	}
	elems := map[string]tftypes.Value{}
	for k, v := range m {
		elems[k] = tftypes.NewValue(tftypes.String, v)
	}
	return tftypes.NewValue(mt, elems)
}

// instanceVariant returns a full attribute map for slug "vm1-abc" with the given
// mutable fields, suitable for both plan and state in update tests.
func instanceVariant(t *testing.T, name, plan, billing, userData string, tags map[string]string) map[string]tftypes.Value {
	t.Helper()
	v := instanceValues(t, "vm1-abc")
	v["name"] = tftypes.NewValue(tftypes.String, name)
	v["plan"] = tftypes.NewValue(tftypes.String, plan)
	v["billing_cycle"] = tftypes.NewValue(tftypes.String, billing)
	v["user_data"] = strVal(userData)
	v["tags"] = tagsVal(tags)
	return v
}

func updateInstance(t *testing.T, svc *fakeInstanceService, planMap, stateMap map[string]tftypes.Value) resource.UpdateResponse {
	t.Helper()
	r := internalprovider.NewInstanceResourceWithService(svc)
	schResp := instanceSchema(t)
	tfType := instanceTFType(t)
	updateReq := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, planMap)},
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, stateMap)},
	}
	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, stateMap)}}
	r.Update(context.Background(), updateReq, updateResp)
	return *updateResp
}

// TestInstanceResource_updateResizeIsTransparent verifies that changing the plan
// stops the (running) VM, changes the offering, and restarts it back to running —
// without the user touching power_state.
func TestInstanceResource_updateResizeIsTransparent(t *testing.T) {
	vm := &instance.VirtualMachine{Slug: "vm1-abc", Name: "vm1", State: "Running"}
	svc := &fakeInstanceService{got: vm}
	state := instanceVariant(t, "vm1", "ci1.small", "hourly", "", nil)
	plan := instanceVariant(t, "vm2", "ci1.large", "hourly", "echo hi", nil)
	resp := updateInstance(t, svc, plan, state)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if svc.renamedTo != "vm2" {
		t.Errorf("renamedTo = %q, want vm2 (display name updated in place)", svc.renamedTo)
	}
	if svc.changedPlan == nil || svc.changedPlan.Plan != "ci1.large" {
		t.Errorf("changedPlan = %+v, want plan ci1.large", svc.changedPlan)
	}
	if svc.changedUD == nil || *svc.changedUD != "echo hi" {
		t.Errorf("changedUD = %v, want 'echo hi'", svc.changedUD)
	}
	// Transparent resize: stopped once for the offering change, started back.
	if svc.stopCalled != 1 {
		t.Errorf("stopCalled = %d, want 1 (resize stops the VM)", svc.stopCalled)
	}
	if svc.startCalled != 1 {
		t.Errorf("startCalled = %d, want 1 (resize restarts the VM)", svc.startCalled)
	}
	if vm.State != "Running" {
		t.Errorf("final VM state = %q, want Running", vm.State)
	}
}

// TestInstanceResource_updateResizeWhenStopped verifies that resizing a VM that
// is already stopped does not start it — power state is preserved, not managed.
func TestInstanceResource_updateResizeWhenStopped(t *testing.T) {
	vm := &instance.VirtualMachine{Slug: "vm1-abc", State: "Stopped"}
	svc := &fakeInstanceService{got: vm}
	state := instanceVariant(t, "vm1", "ci1.small", "hourly", "", nil)
	plan := instanceVariant(t, "vm1", "ci1.large", "hourly", "", nil)
	resp := updateInstance(t, svc, plan, state)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if svc.changedPlan == nil || svc.changedPlan.Plan != "ci1.large" {
		t.Errorf("changedPlan = %+v, want plan ci1.large", svc.changedPlan)
	}
	// Already stopped → no stop and no start; the VM stays stopped.
	if svc.stopCalled != 0 || svc.startCalled != 0 {
		t.Errorf("power toggled for an already-stopped VM: stop=%d start=%d", svc.stopCalled, svc.startCalled)
	}
	if vm.State != "Stopped" {
		t.Errorf("final VM state = %q, want Stopped", vm.State)
	}
}

// TestInstanceResource_resizeRestartsOnFailure verifies that a failed offering
// change restarts a VM that was running, honoring the transparent-resize contract.
// TestInstanceResource_resizeViaActivityLog verifies a resize completes when the
// cached state never updates (stays "Running"), relying on the live /meta view
// to confirm each stop/change/start step. Without it this would hang.
func TestInstanceResource_resizeWithStaleCachedState(t *testing.T) {
	vm := &instance.VirtualMachine{Slug: "vm1-abc", State: "Running"}
	svc := &fakeInstanceService{got: vm, staleCache: true}
	state := instanceVariant(t, "vm1", "ci1.small", "hourly", "", nil)
	plan := instanceVariant(t, "vm1", "ci1.large", "hourly", "", nil)
	resp := updateInstance(t, svc, plan, state)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if svc.stopCalled != 1 || svc.startCalled != 1 {
		t.Errorf("stop=%d start=%d, want 1/1 (transparent resize stop+restart)", svc.stopCalled, svc.startCalled)
	}
	if svc.changedPlan == nil || svc.changedPlan.Plan != "ci1.large" {
		t.Errorf("ChangePlan not called with new plan: %+v", svc.changedPlan)
	}
}

func TestInstanceResource_resizeRestartsOnFailure(t *testing.T) {
	vm := &instance.VirtualMachine{Slug: "vm1-abc", State: "Running"}
	svc := &fakeInstanceService{got: vm, changePlanErr: errors.New("offering change failed")}
	state := instanceVariant(t, "vm1", "ci1.small", "hourly", "", nil)
	plan := instanceVariant(t, "vm1", "ci1.large", "hourly", "", nil)
	resp := updateInstance(t, svc, plan, state)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when ChangePlan fails")
	}
	if svc.stopCalled != 1 {
		t.Errorf("stopCalled = %d, want 1 (stopped for resize)", svc.stopCalled)
	}
	if svc.startCalled != 1 {
		t.Errorf("startCalled = %d, want 1 (restarted after failed resize)", svc.startCalled)
	}
}

// TestInstanceResource_createCleansUpOnWaitFailure verifies that a VM which never
// reaches Running is deleted so it is not orphaned outside Terraform state.
func TestInstanceResource_createCleansUpOnWaitFailure(t *testing.T) {
	svc := &fakeInstanceService{
		created: &instance.VirtualMachine{Slug: "vm1-abc", State: "Pending"},
		got:     &instance.VirtualMachine{Slug: "vm1-abc", State: "Failed"},
	}
	resp := createInstance(t, svc)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when instance fails to provision")
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "vm1-abc" {
		t.Errorf("cleanup Delete called with %v, want [vm1-abc]", svc.deleted)
	}
	if len(svc.expunged) != 1 || !svc.expunged[0] {
		t.Errorf("cleanup Delete expunge = %v, want [true]", svc.expunged)
	}
	if len(svc.deletedPublicIP) != 1 || !svc.deletedPublicIP[0] {
		t.Errorf("cleanup Delete deletePublicIP = %v, want [true]", svc.deletedPublicIP)
	}
}

func TestInstanceResource_updateTags(t *testing.T) {
	vm := &instance.VirtualMachine{Slug: "vm1-abc", State: "Running"}
	svc := &fakeInstanceService{got: vm}
	state := instanceVariant(t, "vm1", "ci1.small", "hourly", "", map[string]string{"A": "1", "C": "9"})
	plan := instanceVariant(t, "vm1", "ci1.small", "hourly", "", map[string]string{"A": "2", "B": "3"})
	resp := updateInstance(t, svc, plan, state)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if svc.tagsCreated["A"] != "2" || svc.tagsCreated["B"] != "3" {
		t.Errorf("tagsCreated = %v, want A:2 B:3", svc.tagsCreated)
	}
	if len(svc.tagsDeleted) != 1 || svc.tagsDeleted[0] != "C" {
		t.Errorf("tagsDeleted = %v, want [C]", svc.tagsDeleted)
	}
	// No resize → no power toggling.
	if svc.stopCalled != 0 || svc.startCalled != 0 {
		t.Errorf("power toggled unexpectedly: stop=%d start=%d", svc.stopCalled, svc.startCalled)
	}
}

func readInstance(t *testing.T, svc *fakeInstanceService, slug string) resource.ReadResponse {
	t.Helper()
	r := internalprovider.NewInstanceResourceWithService(svc)
	schResp := instanceSchema(t)
	tfType := instanceTFType(t)
	stateVal := tftypes.NewValue(tfType, instanceValues(t, slug))
	readReq := resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), readReq, readResp)
	return *readResp
}

func TestInstanceResource_readFound(t *testing.T) {
	svc := &fakeInstanceService{
		got: &instance.VirtualMachine{Slug: "vm1-abc", Name: "vm1", State: "Running"},
	}
	resp := readInstance(t, svc, "vm1-abc")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got instanceStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.State.ValueString() != "Running" {
		t.Errorf("State = %q, want Running", got.State.ValueString())
	}
}

func TestInstanceResource_readNotFound(t *testing.T) {
	svc := &fakeInstanceService{getErr: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := readInstance(t, svc, "gone")
	if resp.Diagnostics.HasError() {
		t.Fatalf("not-found read should not error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected null state after RemoveResource")
	}
}

func deleteInstance(t *testing.T, svc *fakeInstanceService, slug string) resource.DeleteResponse {
	t.Helper()
	r := internalprovider.NewInstanceResourceWithService(svc)
	schResp := instanceSchema(t)
	tfType := instanceTFType(t)
	stateVal := tftypes.NewValue(tfType, instanceValues(t, slug))
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

func TestInstanceResource_deleteHappyPath(t *testing.T) {
	// Get returns 404 immediately so pollUntilGone sees it as gone.
	svc := &fakeInstanceService{getErr: &apierrors.APIError{StatusCode: 404}}
	resp := deleteInstance(t, svc, "vm1-abc")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "vm1-abc" {
		t.Errorf("Delete called with %v, want [vm1-abc]", svc.deleted)
	}
	// assign_public_ip is unset in state → defaults to true, so destroy must
	// release the auto-assigned public IP rather than strand it.
	if len(svc.deletedPublicIP) != 1 || !svc.deletedPublicIP[0] {
		t.Errorf("Delete deletePublicIP = %v, want [true]", svc.deletedPublicIP)
	}
}

func TestInstanceResource_deleteKeepsIPWhenAssignPublicIPFalse(t *testing.T) {
	// assign_public_ip=false means no auto-assigned IP exists, so destroy must
	// not ask the API to release one.
	svc := &fakeInstanceService{getErr: &apierrors.APIError{StatusCode: 404}}
	r := internalprovider.NewInstanceResourceWithService(svc)
	schResp := instanceSchema(t)
	vals := instanceValues(t, "vm1-abc")
	vals["assign_public_ip"] = tftypes.NewValue(tftypes.Bool, false)
	stateVal := tftypes.NewValue(instanceTFType(t), vals)
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", deleteResp.Diagnostics)
	}
	if len(svc.deletedPublicIP) != 1 || svc.deletedPublicIP[0] {
		t.Errorf("Delete deletePublicIP = %v, want [false]", svc.deletedPublicIP)
	}
}

func TestInstanceResource_delete404IsNoOp(t *testing.T) {
	svc := &fakeInstanceService{
		deleteErr: &apierrors.APIError{StatusCode: 404},
		getErr:    &apierrors.APIError{StatusCode: 404},
	}
	resp := deleteInstance(t, svc, "gone")
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 delete should be a no-op: %v", resp.Diagnostics)
	}
}

// fakeIPLister satisfies publicIPLister.
type fakeIPLister struct {
	ips  []ipaddress.IPAddress
	err  error
	seen int
}

func (f *fakeIPLister) List(_ context.Context, _, _, _ string) ([]ipaddress.IPAddress, error) {
	f.seen++
	return f.ips, f.err
}

// TestInstanceResource_createWaitsForPrivateIP verifies the post-Running wait:
// the first Get after meta reports Running has no address yet, the second one
// does, and the applied state carries it.
func TestInstanceResource_createWaitsForPrivateIP(t *testing.T) {
	noIP := &instance.VirtualMachine{Slug: "vm1-abc", State: "Starting"}
	withIP := &instance.VirtualMachine{
		Slug: "vm1-abc", State: "Starting",
		Networks: []instance.VMNetwork{{IsDefault: true, Pivot: &instance.VMNetworkIP{IsDefault: true, IPAddress: "10.0.0.42"}}},
	}
	svc := &fakeInstanceService{
		created:   &instance.VirtualMachine{Slug: "vm1-abc", State: "Starting"},
		metaState: "Running",
		gotQueue:  []*instance.VirtualMachine{noIP, withIP},
		got:       withIP,
	}
	resp := createInstance(t, svc)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got instanceStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.PrivateIP.ValueString() != "10.0.0.42" {
		t.Errorf("PrivateIP = %q, want 10.0.0.42 (from the second read)", got.PrivateIP.ValueString())
	}
}

// TestInstanceResource_createNoPrivateIPStillSucceeds verifies the wait is best
// effort: a VM that never reports an address still applies cleanly.
func TestInstanceResource_createNoPrivateIPStillSucceeds(t *testing.T) {
	svc := &fakeInstanceService{
		created:   &instance.VirtualMachine{Slug: "vm1-abc", State: "Starting"},
		metaState: "Running",
		got:       &instance.VirtualMachine{Slug: "vm1-abc", State: "Starting"},
	}
	resp := createInstance(t, svc)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
}

// TestInstanceResource_publicIPResolvedFromIPList verifies the source-NAT
// fallback: the VM object has no public_ip, so it is resolved from the IP list
// by VM ID, preferring a static assignment over source-NAT.
func TestInstanceResource_publicIPResolvedFromIPList(t *testing.T) {
	vm := &instance.VirtualMachine{
		ID: "vm-uuid-1", Slug: "vm1-abc", State: "Running",
		Networks: []instance.VMNetwork{{IsDefault: true, Pivot: &instance.VMNetworkIP{IsDefault: true, IPAddress: "10.0.0.7"}}},
	}
	ipSvc := &fakeIPLister{ips: []ipaddress.IPAddress{
		{VirtualMachineID: "other-vm", IPAddress: "203.0.113.1", Strategy: "STATIC"},
		{VirtualMachineID: "vm-uuid-1", IPAddress: "206.248.159.158", Strategy: "SOURCE-NAT"},
	}}
	svc := &fakeInstanceService{created: vm, got: vm, metaState: "Running"}
	r := internalprovider.NewInstanceResourceWithServices(svc, ipSvc)
	schResp := instanceSchema(t)
	tfType := instanceTFType(t)
	createReq := resource.CreateRequest{Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, instanceValues(t, ""))}}
	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)}}
	r.Create(context.Background(), createReq, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", createResp.Diagnostics)
	}
	var got instanceStateModel
	if diags := createResp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.PublicIP.ValueString() != "206.248.159.158" {
		t.Errorf("PublicIP = %q, want 206.248.159.158 (resolved from IP list)", got.PublicIP.ValueString())
	}
}

// TestInstanceResource_publicIPLookupSkippedWhenOptedOut verifies a private-only
// instance (assign_public_ip = false) never queries the IP list.
func TestInstanceResource_publicIPLookupSkippedWhenOptedOut(t *testing.T) {
	vm := &instance.VirtualMachine{ID: "vm-uuid-1", Slug: "vm1-abc", State: "Running"}
	ipSvc := &fakeIPLister{ips: []ipaddress.IPAddress{{VirtualMachineID: "vm-uuid-1", IPAddress: "206.248.159.158", Strategy: "SOURCE-NAT"}}}
	svc := &fakeInstanceService{created: vm, got: vm, metaState: "Running"}
	r := internalprovider.NewInstanceResourceWithServices(svc, ipSvc)
	schResp := instanceSchema(t)
	vals := instanceValues(t, "")
	vals["assign_public_ip"] = tftypes.NewValue(tftypes.Bool, false)
	createReq := resource.CreateRequest{Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: tftypes.NewValue(instanceTFType(t), vals)}}
	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(instanceTFType(t), nil)}}
	r.Create(context.Background(), createReq, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", createResp.Diagnostics)
	}
	if ipSvc.seen != 0 {
		t.Errorf("IP list queried %d times for a private-only instance, want 0", ipSvc.seen)
	}
	var got instanceStateModel
	if diags := createResp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.PublicIP.ValueString() != "" {
		t.Errorf("PublicIP = %q, want empty for a private-only instance", got.PublicIP.ValueString())
	}
}
