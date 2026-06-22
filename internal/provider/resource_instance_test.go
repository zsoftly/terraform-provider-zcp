package provider_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/instance"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeInstanceService satisfies instanceServiceIface.
type fakeInstanceService struct {
	created   *instance.VirtualMachine
	waited    *instance.VirtualMachine
	got       *instance.VirtualMachine
	createErr error
	waitErr   error
	getErr    error
	deleted   []string
	deleteErr error
	getCalls  int

	// update-path capture
	renamedTo    string
	changedPlan  *instance.ChangePlanRequest
	changedUD    *string
	tagsCreated  map[string]string
	tagsDeleted  []string
	startCalled  int
	stopCalled   int
	waitedStates [][]string
}

func (f *fakeInstanceService) Create(_ context.Context, _ instance.CreateRequest) (*instance.VirtualMachine, error) {
	return f.created, f.createErr
}
func (f *fakeInstanceService) Get(_ context.Context, _ string) (*instance.VirtualMachine, error) {
	f.getCalls++
	return f.got, f.getErr
}
func (f *fakeInstanceService) WaitForState(_ context.Context, _ string, states []string, _ time.Duration) (*instance.VirtualMachine, error) {
	f.waitedStates = append(f.waitedStates, states)
	return f.waited, f.waitErr
}
func (f *fakeInstanceService) ChangeHostname(_ context.Context, _ string, req instance.ChangeLabelRequest) (*instance.ActionResponse, error) {
	f.renamedTo = req.Name
	return &instance.ActionResponse{}, nil
}
func (f *fakeInstanceService) ChangePlan(_ context.Context, _ string, req instance.ChangePlanRequest) (*instance.ActionResponse, error) {
	f.changedPlan = &req
	return &instance.ActionResponse{}, nil
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
	if f.got != nil {
		f.got.State = "Running"
	}
	return &instance.ActionResponse{}, nil
}
func (f *fakeInstanceService) Stop(_ context.Context, _ string) (*instance.ActionResponse, error) {
	f.stopCalled++
	if f.got != nil {
		f.got.State = "Stopped"
	}
	return &instance.ActionResponse{}, nil
}
func (f *fakeInstanceService) Delete(_ context.Context, slug string, _ bool) error {
	f.deleted = append(f.deleted, slug)
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
	NetworkPlan     types.String   `tfsdk:"network_plan"`
	StorageCategory types.String   `tfsdk:"storage_category"`
	UserData        types.String   `tfsdk:"user_data"`
	Tags            types.Map      `tfsdk:"tags"`
	PowerState      types.String   `tfsdk:"power_state"`
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
		"network_plan":     null(),
		"storage_category": null(),
		"user_data":        null(),
		"tags":             tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, nil),
		"power_state":      null(),
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
	svc := &fakeInstanceService{
		created: &instance.VirtualMachine{Slug: "vm1-abc", State: "Pending"},
		waited: &instance.VirtualMachine{
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

func TestInstanceResource_createWaitError(t *testing.T) {
	svc := &fakeInstanceService{
		created: &instance.VirtualMachine{Slug: "vm1-abc", State: "Pending"},
		waitErr: errors.New("timed out"),
	}
	resp := createInstance(t, svc)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when wait fails")
	}
}

func TestInstanceResource_createServiceError(t *testing.T) {
	svc := &fakeInstanceService{createErr: errors.New("quota exceeded")}
	resp := createInstance(t, svc)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure")
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
func instanceVariant(t *testing.T, name, plan, billing, userData, power string, tags map[string]string) map[string]tftypes.Value {
	t.Helper()
	v := instanceValues(t, "vm1-abc")
	v["name"] = tftypes.NewValue(tftypes.String, name)
	v["plan"] = tftypes.NewValue(tftypes.String, plan)
	v["billing_cycle"] = tftypes.NewValue(tftypes.String, billing)
	v["user_data"] = strVal(userData)
	v["power_state"] = strVal(power)
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
	vm := &instance.VirtualMachine{Slug: "vm1-abc", Name: "vm2", State: "Running"}
	svc := &fakeInstanceService{got: vm, waited: vm}
	state := instanceVariant(t, "vm1", "ci1.small", "hourly", "", "running", nil)
	plan := instanceVariant(t, "vm2", "ci1.large", "hourly", "echo hi", "running", nil)
	resp := updateInstance(t, svc, plan, state)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if svc.renamedTo != "vm2" {
		t.Errorf("renamedTo = %q, want vm2", svc.renamedTo)
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

func TestInstanceResource_updateStop(t *testing.T) {
	vm := &instance.VirtualMachine{Slug: "vm1-abc", State: "Running"}
	svc := &fakeInstanceService{got: vm, waited: vm}
	state := instanceVariant(t, "vm1", "ci1.small", "hourly", "", "running", nil)
	plan := instanceVariant(t, "vm1", "ci1.small", "hourly", "", "stopped", nil)
	resp := updateInstance(t, svc, plan, state)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if svc.stopCalled != 1 {
		t.Errorf("stopCalled = %d, want 1", svc.stopCalled)
	}
	if svc.startCalled != 0 {
		t.Errorf("startCalled = %d, want 0", svc.startCalled)
	}
	if vm.State != "Stopped" {
		t.Errorf("final VM state = %q, want Stopped", vm.State)
	}
}

func TestInstanceResource_updateTags(t *testing.T) {
	vm := &instance.VirtualMachine{Slug: "vm1-abc", State: "Running"}
	svc := &fakeInstanceService{got: vm, waited: vm}
	state := instanceVariant(t, "vm1", "ci1.small", "hourly", "", "running", map[string]string{"A": "1", "C": "9"})
	plan := instanceVariant(t, "vm1", "ci1.small", "hourly", "", "running", map[string]string{"A": "2", "B": "3"})
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
	// No resize and already running → no power toggling.
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
