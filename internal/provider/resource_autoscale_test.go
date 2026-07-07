package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/autoscale"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeAutoscaleService satisfies autoscaleServiceIface.
type fakeAutoscaleService struct {
	groups            []autoscale.AutoscaleGroup
	created           *autoscale.AutoscaleGroup
	createReq         autoscale.CreateRequest
	err               error
	disableErr        error
	deleted           []string
	planChanges       []string
	templateChanges   []string
	enabled           int
	disabled          int
	createdPolicy     *autoscale.Policy
	policyUpdates     []autoscale.PolicyRequest
	policyDeletes     []int
	createdCondition  *autoscale.Condition
	conditionUpdates  []autoscale.ConditionRequest
	conditionDeletes  []int
	lastPolicyRequest autoscale.PolicyRequest
}

func (f *fakeAutoscaleService) List(_ context.Context, _, _ string) ([]autoscale.AutoscaleGroup, error) {
	return f.groups, f.err
}
func (f *fakeAutoscaleService) Create(_ context.Context, req autoscale.CreateRequest) (*autoscale.AutoscaleGroup, error) {
	f.createReq = req
	return f.created, f.err
}
func (f *fakeAutoscaleService) Delete(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	if f.err == nil {
		f.groups = nil
	}
	return f.err
}
func (f *fakeAutoscaleService) ChangePlan(_ context.Context, _, plan string) (*autoscale.AutoscaleGroup, error) {
	f.planChanges = append(f.planChanges, plan)
	return &autoscale.AutoscaleGroup{}, f.err
}
func (f *fakeAutoscaleService) ChangeTemplate(_ context.Context, _, template string) (*autoscale.AutoscaleGroup, error) {
	f.templateChanges = append(f.templateChanges, template)
	return &autoscale.AutoscaleGroup{}, f.err
}
func (f *fakeAutoscaleService) Enable(_ context.Context, _ string) (*autoscale.AutoscaleGroup, error) {
	f.enabled++
	return &autoscale.AutoscaleGroup{}, f.err
}
func (f *fakeAutoscaleService) Disable(_ context.Context, _ string) (*autoscale.AutoscaleGroup, error) {
	f.disabled++
	if f.disableErr != nil {
		return nil, f.disableErr
	}
	return &autoscale.AutoscaleGroup{}, f.err
}
func (f *fakeAutoscaleService) CreatePolicy(_ context.Context, _ string, req autoscale.PolicyRequest) (*autoscale.Policy, error) {
	f.lastPolicyRequest = req
	return f.createdPolicy, f.err
}
func (f *fakeAutoscaleService) UpdatePolicy(_ context.Context, _ string, _ int, req autoscale.PolicyRequest) (*autoscale.Policy, error) {
	f.policyUpdates = append(f.policyUpdates, req)
	return &autoscale.Policy{}, f.err
}
func (f *fakeAutoscaleService) DeletePolicy(_ context.Context, _ string, policyID int) error {
	f.policyDeletes = append(f.policyDeletes, policyID)
	return f.err
}
func (f *fakeAutoscaleService) CreateCondition(_ context.Context, _ string, req autoscale.ConditionRequest) (*autoscale.Condition, error) {
	return f.createdCondition, f.err
}
func (f *fakeAutoscaleService) UpdateCondition(_ context.Context, _ string, _ int, req autoscale.ConditionRequest) (*autoscale.Condition, error) {
	f.conditionUpdates = append(f.conditionUpdates, req)
	return &autoscale.Condition{}, f.err
}
func (f *fakeAutoscaleService) DeleteCondition(_ context.Context, _ string, conditionID int) error {
	f.conditionDeletes = append(f.conditionDeletes, conditionID)
	return f.err
}

func intVal(v int64) tftypes.Value { return tftypes.NewValue(tftypes.Number, v) }

func autoscaleGroupConfig(id string) map[string]tftypes.Value {
	cfg := map[string]tftypes.Value{
		"name":           strVal("web-asg"),
		"plan":           strVal("ci1xs"),
		"template":       strVal("ubuntu-24"),
		"min_instances":  intVal(1),
		"max_instances":  intVal(5),
		"zone":           strVal("yow-zone-1"),
		"cloud_provider": strVal("zsoftly"),
		"region":         strVal("yow-1"),
	}
	if id != "" {
		cfg["id"] = strVal(id)
	}
	return cfg
}

func TestAutoscaleGroupResource_createHappyPath(t *testing.T) {
	svc := &fakeAutoscaleService{
		created: &autoscale.AutoscaleGroup{
			Slug: "web-asg-a1", Name: "web-asg", State: "enabled",
			Plan: "ci1xs", Template: "ubuntu-24", MinInstances: 1, MaxInstances: 5,
			ZoneSlug: "yow-zone-1", CurrentCount: 1,
		},
	}
	r := internalprovider.NewAutoscaleGroupResourceWithService(svc)
	resp := runCreate(t, r, autoscaleGroupConfig(""))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := stateID(t, resp.State); got != "web-asg-a1" {
		t.Errorf("ID = %q, want web-asg-a1", got)
	}
	if svc.createReq.MinInstances != 1 || svc.createReq.MaxInstances != 5 {
		t.Errorf("createReq bounds = %d/%d, want 1/5", svc.createReq.MinInstances, svc.createReq.MaxInstances)
	}
	if svc.disabled != 0 {
		t.Errorf("Disable called %d times on plain create, want 0", svc.disabled)
	}
}

func TestAutoscaleGroupResource_createDisableFailureSurfaces(t *testing.T) {
	// enabled=false disables right after create; when that disable fails the
	// resource must error (tainting the group) and record it as still enabled
	// rather than pretending the disable succeeded.
	svc := &fakeAutoscaleService{
		created: &autoscale.AutoscaleGroup{
			Slug: "web-asg-a1", Name: "web-asg", State: "enabled",
			Plan: "ci1xs", Template: "ubuntu-24", MinInstances: 1, MaxInstances: 5,
			ZoneSlug: "yow-zone-1", CurrentCount: 1,
		},
		disableErr: errors.New("api down"),
	}
	r := internalprovider.NewAutoscaleGroupResourceWithService(svc)
	cfg := autoscaleGroupConfig("")
	cfg["enabled"] = tftypes.NewValue(tftypes.Bool, false)
	resp := runCreate(t, r, cfg)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when the post-create disable fails, got none")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("expected partial state so the group is tracked and tainted, got none")
	}
	var enabled types.Bool
	if diags := resp.State.GetAttribute(context.Background(), path.Root("enabled"), &enabled); diags.HasError() {
		t.Fatalf("reading enabled: %v", diags)
	}
	if enabled.IsNull() || !enabled.ValueBool() {
		t.Errorf("state enabled = %v, want true (the group is really still enabled)", enabled)
	}
}

func TestAutoscaleGroupResource_createServiceError(t *testing.T) {
	svc := &fakeAutoscaleService{err: errors.New("no capacity")}
	r := internalprovider.NewAutoscaleGroupResourceWithService(svc)
	resp := runCreate(t, r, autoscaleGroupConfig(""))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func TestAutoscaleGroupResource_updateChangesPlanAndTemplate(t *testing.T) {
	svc := &fakeAutoscaleService{}
	r := internalprovider.NewAutoscaleGroupResourceWithService(svc)
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)

	stateCfg := autoscaleGroupConfig("web-asg-a1")
	planCfg := autoscaleGroupConfig("web-asg-a1")
	planCfg["plan"] = strVal("ci1s")
	planCfg["template"] = strVal("ubuntu-26")

	_, stateVal := rawFor(t, r, stateCfg)
	_, planVal := rawFor(t, r, planCfg)
	updateReq := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schResp.Schema, Raw: planVal},
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Update(context.Background(), updateReq, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", updateResp.Diagnostics)
	}
	if len(svc.planChanges) != 1 || svc.planChanges[0] != "ci1s" {
		t.Errorf("planChanges = %v, want [ci1s]", svc.planChanges)
	}
	if len(svc.templateChanges) != 1 || svc.templateChanges[0] != "ubuntu-26" {
		t.Errorf("templateChanges = %v, want [ubuntu-26]", svc.templateChanges)
	}
}

func TestAutoscaleGroupResource_updateDisables(t *testing.T) {
	svc := &fakeAutoscaleService{}
	r := internalprovider.NewAutoscaleGroupResourceWithService(svc)
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)

	stateCfg := autoscaleGroupConfig("web-asg-a1")
	planCfg := autoscaleGroupConfig("web-asg-a1")
	planCfg["enabled"] = tftypes.NewValue(tftypes.Bool, false)

	_, stateVal := rawFor(t, r, stateCfg)
	_, planVal := rawFor(t, r, planCfg)
	updateReq := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schResp.Schema, Raw: planVal},
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Update(context.Background(), updateReq, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", updateResp.Diagnostics)
	}
	if svc.disabled != 1 {
		t.Errorf("Disable called %d times, want 1", svc.disabled)
	}
}

func TestAutoscaleGroupResource_deleteHappyPath(t *testing.T) {
	svc := &fakeAutoscaleService{
		groups: []autoscale.AutoscaleGroup{{Slug: "web-asg-a1"}},
	}
	r := internalprovider.NewAutoscaleGroupResourceWithService(svc)
	resp := runDelete(t, r, autoscaleGroupConfig("web-asg-a1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "web-asg-a1" {
		t.Errorf("Delete called with %v, want [web-asg-a1]", svc.deleted)
	}
}

// --- zcp_autoscale_policy / zcp_autoscale_condition ---

func autoscaleRuleConfig(id string) map[string]tftypes.Value {
	cfg := map[string]tftypes.Value{
		"autoscale_group": strVal("web-asg-a1"),
		"name":            strVal("cpu-high"),
		"metric":          strVal("cpu"),
		"operator":        strVal("GT"),
		"threshold":       intVal(80),
		"duration":        intVal(300),
		"scale_amount":    intVal(1),
	}
	if id != "" {
		cfg["id"] = strVal(id)
	}
	return cfg
}

func TestAutoscalePolicyResource_createHappyPath(t *testing.T) {
	svc := &fakeAutoscaleService{
		createdPolicy: &autoscale.Policy{ID: "11", Name: "cpu-high"},
	}
	r := internalprovider.NewAutoscalePolicyResourceWithService(svc)
	resp := runCreate(t, r, autoscaleRuleConfig(""))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := stateID(t, resp.State); got != "11" {
		t.Errorf("ID = %q, want 11", got)
	}
	if svc.lastPolicyRequest.Threshold != 80 {
		t.Errorf("policy threshold = %d, want 80", svc.lastPolicyRequest.Threshold)
	}
}

func TestAutoscalePolicyResource_updateInPlace(t *testing.T) {
	svc := &fakeAutoscaleService{}
	r := internalprovider.NewAutoscalePolicyResourceWithService(svc)
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)

	stateCfg := autoscaleRuleConfig("11")
	planCfg := autoscaleRuleConfig("11")
	planCfg["threshold"] = intVal(90)

	_, stateVal := rawFor(t, r, stateCfg)
	_, planVal := rawFor(t, r, planCfg)
	updateReq := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schResp.Schema, Raw: planVal},
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Update(context.Background(), updateReq, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", updateResp.Diagnostics)
	}
	if len(svc.policyUpdates) != 1 || svc.policyUpdates[0].Threshold != 90 {
		t.Errorf("policyUpdates = %+v, want one update with threshold 90", svc.policyUpdates)
	}
}

func TestAutoscalePolicyResource_readGoneRemoves(t *testing.T) {
	svc := &fakeAutoscaleService{
		groups: []autoscale.AutoscaleGroup{{Slug: "web-asg-a1"}}, // no policies
	}
	r := internalprovider.NewAutoscalePolicyResourceWithService(svc)
	resp := runRead(t, r, autoscaleRuleConfig("11"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestAutoscaleConditionResource_createAndDelete(t *testing.T) {
	svc := &fakeAutoscaleService{
		createdCondition: &autoscale.Condition{ID: "21", Name: "cpu-low"},
	}
	r := internalprovider.NewAutoscaleConditionResourceWithService(svc)
	cfg := autoscaleRuleConfig("")
	cfg["name"] = strVal("cpu-low")
	cfg["operator"] = strVal("LT")
	cfg["threshold"] = intVal(20)
	resp := runCreate(t, r, cfg)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := stateID(t, resp.State); got != "21" {
		t.Errorf("ID = %q, want 21", got)
	}

	delResp := runDelete(t, r, autoscaleRuleConfig("21"))
	if delResp.Diagnostics.HasError() {
		t.Fatalf("unexpected delete error: %v", delResp.Diagnostics)
	}
	if len(svc.conditionDeletes) != 1 || svc.conditionDeletes[0] != 21 {
		t.Errorf("conditionDeletes = %v, want [21]", svc.conditionDeletes)
	}
}
