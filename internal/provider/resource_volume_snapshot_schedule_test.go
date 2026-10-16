package provider_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/scheduler"
	"github.com/zsoftly/zcp-cli/pkg/api/volume"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

type fakeVolumeSnapshotScheduleService struct {
	policy                   scheduler.Policy
	created                  scheduler.CreateRequest
	updated                  scheduler.UpdateRequest
	updatePolicy             *scheduler.Policy
	paused, resumed, deleted bool
	getErr                   error
	pausePolicy              *scheduler.Policy
	resumePolicy             *scheduler.Policy
}

func (f *fakeVolumeSnapshotScheduleService) Get(_ context.Context, _ string) (*scheduler.Policy, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &f.policy, nil
}
func (f *fakeVolumeSnapshotScheduleService) Create(_ context.Context, req scheduler.CreateRequest) (*scheduler.Policy, error) {
	f.created = req
	return &f.policy, nil
}
func (f *fakeVolumeSnapshotScheduleService) Update(_ context.Context, _ string, req scheduler.UpdateRequest) (*scheduler.Policy, error) {
	f.updated = req
	if f.updatePolicy != nil {
		return f.updatePolicy, nil
	}
	return &f.policy, nil
}
func (f *fakeVolumeSnapshotScheduleService) Delete(_ context.Context, _ string) (*scheduler.ActionResponse, error) {
	f.deleted = true
	return &scheduler.ActionResponse{Status: "Success"}, nil
}
func (f *fakeVolumeSnapshotScheduleService) Pause(_ context.Context, _ string) (*scheduler.Policy, error) {
	f.paused = true
	if f.pausePolicy != nil {
		return f.pausePolicy, nil
	}
	f.policy.Status = "paused"
	return &f.policy, nil
}
func (f *fakeVolumeSnapshotScheduleService) Resume(_ context.Context, _ string) (*scheduler.Policy, error) {
	f.resumed = true
	if f.resumePolicy != nil {
		return f.resumePolicy, nil
	}
	f.policy.Status = "active"
	return &f.policy, nil
}

type fakeVolumeSnapshotScheduleVolumes struct {
	volumes []volume.Volume
	err     error
}

func (f *fakeVolumeSnapshotScheduleVolumes) List(_ context.Context, _, _ string) ([]volume.Volume, error) {
	return f.volumes, f.err
}

func volumeSnapshotSchedulePolicy(action string) scheduler.Policy {
	day := 2
	return scheduler.Policy{ID: "policy-1", Action: action, Actionable: &scheduler.VMRef{Slug: "data-1"}, Region: &scheduler.ScopeRef{Slug: "yow-1"}, Interval: "weeklyOn", Day: &day, At: "02:00", Timezone: "America/Toronto", RetentionPolicy: 3, Status: "active"}
}

func volumeSnapshotScheduleConfig(id string, paused bool) map[string]tftypes.Value {
	config := map[string]tftypes.Value{
		"volume": strVal("data-1"), "interval": strVal("weeklyOn"), "day": intVal(2),
		"at": strVal("02:00"), "timezone": strVal("America/Toronto"), "retention": intVal(3),
		"region": strVal("yow-1"), "paused": tftypes.NewValue(tftypes.Bool, paused),
	}
	if id != "" {
		config["id"] = strVal(id)
	}
	return config
}

func TestVolumeSnapshotScheduleResourceCRUDPayloadAndPause(t *testing.T) {
	svc := &fakeVolumeSnapshotScheduleService{policy: volumeSnapshotSchedulePolicy("Block Storage Snapshot")}
	volumes := &fakeVolumeSnapshotScheduleVolumes{volumes: []volume.Volume{{Slug: "data-1"}}}
	r := internalprovider.NewVolumeSnapshotScheduleResourceWithService(svc, volumes)

	create := runCreate(t, r, volumeSnapshotScheduleConfig("", true))
	if create.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", create.Diagnostics)
	}
	if svc.created.Service != "Block Storage" || svc.created.Action != "Block Storage Snapshot" || svc.created.Slug != "data-1" || svc.created.Config.TakeOneImmediately != 0 {
		t.Fatalf("unexpected create payload: %#v", svc.created)
	}
	if !svc.paused || stateID(t, create.State) != "policy-1" {
		t.Fatalf("create did not persist paused policy: paused=%t id=%q", svc.paused, stateID(t, create.State))
	}

	update := runUpdate(t, r, volumeSnapshotScheduleConfig("policy-1", false), volumeSnapshotScheduleConfig("policy-1", true))
	if update.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", update.Diagnostics)
	}
	if !svc.resumed || svc.updated.RetentionPolicy != 3 {
		t.Fatalf("update did not resume and update policy: resumed=%t payload=%#v", svc.resumed, svc.updated)
	}

	deleted := runDelete(t, r, volumeSnapshotScheduleConfig("policy-1", false))
	if deleted.Diagnostics.HasError() || !svc.deleted {
		t.Fatalf("delete diagnostics=%v deleted=%t", deleted.Diagnostics, svc.deleted)
	}
}

func TestVolumeSnapshotScheduleResourceCreateRequestsImmediateSnapshot(t *testing.T) {
	svc := &fakeVolumeSnapshotScheduleService{policy: volumeSnapshotSchedulePolicy("Block Storage Snapshot")}
	r := internalprovider.NewVolumeSnapshotScheduleResourceWithService(svc, &fakeVolumeSnapshotScheduleVolumes{volumes: []volume.Volume{{Slug: "data-1"}}})
	config := volumeSnapshotScheduleConfig("", false)
	config["immediate"] = tftypes.NewValue(tftypes.Bool, true)
	resp := runCreate(t, r, config)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}
	if svc.created.TakeOneImmediately != 1 || svc.created.Config.TakeOneImmediately != 1 {
		t.Fatalf("immediate payload: %#v", svc.created)
	}
}

func TestVolumeSnapshotScheduleResourceImmediateAdoptsAfterImport(t *testing.T) {
	r := internalprovider.NewVolumeSnapshotScheduleResource()
	var schemaResponse resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
	attribute := schemaResponse.Schema.Attributes["immediate"].(schema.BoolAttribute)

	cases := []struct {
		name     string
		state    types.Bool
		plan     types.Bool
		replaces bool
	}{
		{name: "imported false", state: types.BoolNull(), plan: types.BoolValue(false)},
		{name: "imported true", state: types.BoolNull(), plan: types.BoolValue(true)},
		{name: "known false to true", state: types.BoolValue(false), plan: types.BoolValue(true), replaces: true},
		{name: "known true to false", state: types.BoolValue(true), plan: types.BoolValue(false), replaces: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stateConfig := volumeSnapshotScheduleConfig("policy-1", false)
			stateConfig["immediate"] = tftypes.NewValue(tftypes.Bool, tc.state.ValueBool())
			if tc.state.IsNull() {
				stateConfig["immediate"] = tftypes.NewValue(tftypes.Bool, nil)
			}
			planConfig := volumeSnapshotScheduleConfig("policy-1", false)
			planConfig["immediate"] = tftypes.NewValue(tftypes.Bool, tc.plan.ValueBool())
			_, stateRaw := rawFor(t, r, stateConfig)
			_, planRaw := rawFor(t, r, planConfig)
			req := planmodifier.BoolRequest{
				Path:        path.Root("immediate"),
				ConfigValue: tc.plan,
				PlanValue:   tc.plan,
				StateValue:  tc.state,
				Plan:        tfsdk.Plan{Schema: schemaResponse.Schema, Raw: planRaw},
				State:       tfsdk.State{Schema: schemaResponse.Schema, Raw: stateRaw},
			}
			resp := planmodifier.BoolResponse{PlanValue: req.PlanValue}
			for _, modifier := range attribute.PlanModifiers {
				modifier.PlanModifyBool(context.Background(), req, &resp)
			}
			if resp.RequiresReplace != tc.replaces {
				t.Fatalf("RequiresReplace = %t, want %t", resp.RequiresReplace, tc.replaces)
			}
		})
	}
}

func TestVolumeSnapshotScheduleResourceDefaultImmediatePersistsAndLaterReplaces(t *testing.T) {
	r := internalprovider.NewVolumeSnapshotScheduleResource()
	var schemaResponse resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
	attribute := schemaResponse.Schema.Attributes["immediate"].(schema.BoolAttribute)
	if attribute.Default == nil {
		t.Fatal("immediate must default to false")
	}
	defaultResponse := &defaults.BoolResponse{}
	attribute.Default.DefaultBool(context.Background(), defaults.BoolRequest{}, defaultResponse)
	if defaultResponse.PlanValue != types.BoolValue(false) {
		t.Fatalf("immediate default = %s, want false", defaultResponse.PlanValue)
	}

	svc := &fakeVolumeSnapshotScheduleService{policy: volumeSnapshotSchedulePolicy("Block Storage Snapshot")}
	configured := volumeSnapshotScheduleConfig("", false)
	configured["immediate"] = tftypes.NewValue(tftypes.Bool, defaultResponse.PlanValue.ValueBool())
	created := runCreate(t, internalprovider.NewVolumeSnapshotScheduleResourceWithService(svc, &fakeVolumeSnapshotScheduleVolumes{volumes: []volume.Volume{{Slug: "data-1"}}}), configured)
	if created.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", created.Diagnostics)
	}
	var immediate *bool
	if diags := created.State.GetAttribute(context.Background(), path.Root("immediate"), &immediate); diags.HasError() {
		t.Fatalf("read immediate state: %v", diags)
	}
	if immediate == nil || *immediate != defaultResponse.PlanValue.ValueBool() {
		t.Fatalf("created immediate state = %v, want %t", immediate, defaultResponse.PlanValue.ValueBool())
	}

	_, planRaw := rawFor(t, r, func() map[string]tftypes.Value {
		config := volumeSnapshotScheduleConfig("policy-1", false)
		config["immediate"] = tftypes.NewValue(tftypes.Bool, true)
		return config
	}())
	req := planmodifier.BoolRequest{
		Path:        path.Root("immediate"),
		ConfigValue: types.BoolValue(true),
		PlanValue:   types.BoolValue(true),
		StateValue:  defaultResponse.PlanValue,
		Plan:        tfsdk.Plan{Schema: schemaResponse.Schema, Raw: planRaw},
		State:       created.State,
	}
	response := planmodifier.BoolResponse{PlanValue: req.PlanValue}
	for _, modifier := range attribute.PlanModifiers {
		modifier.PlanModifyBool(context.Background(), req, &response)
	}
	if !response.RequiresReplace {
		t.Fatal("setting immediate=true after a default false create must require replacement")
	}
}

func TestVolumeSnapshotScheduleResourceUpdatePersistsAdoptedImmediate(t *testing.T) {
	for _, wantImmediate := range []bool{false, true} {
		t.Run(fmt.Sprintf("immediate=%t", wantImmediate), func(t *testing.T) {
			svc := &fakeVolumeSnapshotScheduleService{policy: volumeSnapshotSchedulePolicy("Block Storage Snapshot")}
			r := internalprovider.NewVolumeSnapshotScheduleResourceWithService(svc)
			state := volumeSnapshotScheduleConfig("policy-1", false)
			state["immediate"] = tftypes.NewValue(tftypes.Bool, nil)
			plan := volumeSnapshotScheduleConfig("policy-1", false)
			plan["immediate"] = tftypes.NewValue(tftypes.Bool, wantImmediate)

			resp := runUpdate(t, r, plan, state)
			if resp.Diagnostics.HasError() {
				t.Fatalf("update diagnostics: %v", resp.Diagnostics)
			}
			var immediate *bool
			if diags := resp.State.GetAttribute(context.Background(), path.Root("immediate"), &immediate); diags.HasError() {
				t.Fatalf("read immediate state: %v", diags)
			}
			if immediate == nil || *immediate != wantImmediate {
				t.Fatalf("immediate state = %v, want %t", immediate, wantImmediate)
			}
			if svc.created.Service != "" || svc.deleted {
				t.Fatalf("adoption created or deleted a policy: create=%#v deleted=%t", svc.created, svc.deleted)
			}
		})
	}
}

func TestVolumeSnapshotScheduleResourceRejectsInvalidScheduleConfig(t *testing.T) {
	cases := []struct {
		name  string
		apply func(map[string]tftypes.Value)
	}{
		{name: "interval", apply: func(config map[string]tftypes.Value) { config["interval"] = strVal("everyOtherDay") }},
		{name: "weekday", apply: func(config map[string]tftypes.Value) { config["day"] = intVal(7) }},
		{name: "timezone", apply: func(config map[string]tftypes.Value) { config["timezone"] = strVal("Local") }},
		{name: "retention", apply: func(config map[string]tftypes.Value) { config["retention"] = intVal(0) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := internalprovider.NewVolumeSnapshotScheduleResource().(resource.ResourceWithValidateConfig)
			schemaResponse, raw := rawFor(t, r, func() map[string]tftypes.Value {
				config := volumeSnapshotScheduleConfig("", false)
				tc.apply(config)
				return config
			}())
			var resp resource.ValidateConfigResponse
			r.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schemaResponse.Schema, Raw: raw}}, &resp)
			if !resp.Diagnostics.HasError() {
				t.Fatal("expected validation diagnostic")
			}
		})
	}
}

func TestVolumeSnapshotScheduleResourcePreflightsScopedVolume(t *testing.T) {
	svc := &fakeVolumeSnapshotScheduleService{policy: volumeSnapshotSchedulePolicy("Block Storage Snapshot")}
	for _, volumes := range []*fakeVolumeSnapshotScheduleVolumes{{err: errors.New("inventory unavailable")}, {volumes: nil}} {
		resp := runCreate(t, internalprovider.NewVolumeSnapshotScheduleResourceWithService(svc, volumes), volumeSnapshotScheduleConfig("", false))
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected preflight diagnostic")
		}
		if svc.created.Action != "" {
			t.Fatalf("create request made after failed preflight: %#v", svc.created)
		}
	}
}

func TestVolumeSnapshotScheduleResourceCreateDoesNotPersistOutOfScopePolicy(t *testing.T) {
	policy := volumeSnapshotSchedulePolicy("Block Storage Snapshot")
	policy.Actionable.Slug = "other-volume"
	svc := &fakeVolumeSnapshotScheduleService{policy: policy}
	r := internalprovider.NewVolumeSnapshotScheduleResourceWithService(svc, &fakeVolumeSnapshotScheduleVolumes{volumes: []volume.Volume{{Slug: "data-1"}}})
	resp := runCreate(t, r, volumeSnapshotScheduleConfig("", false))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected out-of-scope create diagnostic")
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("out-of-scope scheduler policy must not be persisted in Terraform state")
	}
	if svc.paused {
		t.Fatal("out-of-scope scheduler policy must not receive follow-up actions")
	}
}

func TestVolumeSnapshotScheduleResourceCreateRetainsCreatedPolicyWhenPauseResponseIsOutOfScope(t *testing.T) {
	invalid := volumeSnapshotSchedulePolicy("Block Storage Snapshot")
	invalid.ID, invalid.Actionable.Slug = "other-policy", "other-volume"
	svc := &fakeVolumeSnapshotScheduleService{policy: volumeSnapshotSchedulePolicy("Block Storage Snapshot"), pausePolicy: &invalid}
	r := internalprovider.NewVolumeSnapshotScheduleResourceWithService(svc, &fakeVolumeSnapshotScheduleVolumes{volumes: []volume.Volume{{Slug: "data-1"}}})
	resp := runCreate(t, r, volumeSnapshotScheduleConfig("", true))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected invalid pause response diagnostic")
	}
	if stateID(t, resp.State) != "policy-1" {
		t.Fatalf("state adopted unowned pause response: id=%q", stateID(t, resp.State))
	}
}

func TestVolumeSnapshotScheduleResourceUpdateRetainsValidatedPolicyWhenStatusResponseIsOutOfScope(t *testing.T) {
	invalid := volumeSnapshotSchedulePolicy("Block Storage Snapshot")
	invalid.ID, invalid.Actionable.Slug = "other-policy", "other-volume"
	policy := volumeSnapshotSchedulePolicy("Block Storage Snapshot")
	policy.Status = "paused"
	updated := volumeSnapshotSchedulePolicy("Block Storage Snapshot")
	updated.Interval, updated.Day, updated.At, updated.RetentionPolicy = "dailyAt", nil, "03:00", 5
	updated.Status = "paused"
	svc := &fakeVolumeSnapshotScheduleService{policy: policy, updatePolicy: &updated, resumePolicy: &invalid}
	r := internalprovider.NewVolumeSnapshotScheduleResourceWithService(svc)
	state := volumeSnapshotScheduleConfig("policy-1", true)
	plan := volumeSnapshotScheduleConfig("policy-1", false)
	plan["interval"] = strVal("dailyAt")
	plan["day"] = tftypes.NewValue(tftypes.Number, nil)
	plan["at"] = strVal("03:00")
	plan["retention"] = intVal(5)
	resp := runUpdate(t, r, plan, state)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected invalid status response diagnostic")
	}
	if stateID(t, resp.State) != "policy-1" {
		t.Fatalf("state adopted unowned status response: id=%q", stateID(t, resp.State))
	}
	var at string
	var retention int64
	if diags := resp.State.GetAttribute(context.Background(), path.Root("at"), &at); diags.HasError() {
		t.Fatalf("read at state: %v", diags)
	}
	if diags := resp.State.GetAttribute(context.Background(), path.Root("retention"), &retention); diags.HasError() {
		t.Fatalf("read retention state: %v", diags)
	}
	if at != "03:00" || retention != 5 {
		t.Fatalf("state kept stale schedule values: at=%q retention=%d", at, retention)
	}
}

func TestVolumeSnapshotScheduleResourceRejectsOtherSchedulerActions(t *testing.T) {
	for _, operation := range []string{"read", "update", "delete"} {
		t.Run(operation, func(t *testing.T) {
			svc := &fakeVolumeSnapshotScheduleService{policy: volumeSnapshotSchedulePolicy("VM Backup")}
			r := internalprovider.NewVolumeSnapshotScheduleResourceWithService(svc)
			state := volumeSnapshotScheduleConfig("policy-1", false)
			switch operation {
			case "read":
				resp := runRead(t, r, state)
				if !resp.Diagnostics.HasError() {
					t.Fatal("expected action guard diagnostic")
				}
			case "update":
				resp := runUpdate(t, r, state, state)
				if !resp.Diagnostics.HasError() {
					t.Fatal("expected action guard diagnostic")
				}
			case "delete":
				resp := runDelete(t, r, state)
				if !resp.Diagnostics.HasError() || svc.deleted {
					t.Fatalf("diagnostics=%v deleted=%t", resp.Diagnostics, svc.deleted)
				}
			}
		})
	}
}

func TestVolumeSnapshotScheduleResourceRejectsWrongScopeBeforeMutation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*scheduler.Policy)
	}{
		{name: "volume", mutate: func(policy *scheduler.Policy) { policy.Actionable.Slug = "other-volume" }},
		{name: "region", mutate: func(policy *scheduler.Policy) { policy.Region.Slug = "yyz-1" }},
		{name: "project", mutate: func(policy *scheduler.Policy) { policy.Project = &scheduler.ScopeRef{Slug: "other-project"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy := volumeSnapshotSchedulePolicy("Block Storage Snapshot")
			tc.mutate(&policy)
			svc := &fakeVolumeSnapshotScheduleService{policy: policy}
			r := internalprovider.NewVolumeSnapshotScheduleResourceWithService(svc)
			state := volumeSnapshotScheduleConfig("policy-1", false)
			if tc.name == "project" {
				state["project"] = strVal("project-1")
			}
			updated := runUpdate(t, r, state, state)
			if !updated.Diagnostics.HasError() || svc.updated.Interval != "" {
				t.Fatalf("update diagnostics=%v payload=%#v", updated.Diagnostics, svc.updated)
			}
			deleted := runDelete(t, r, state)
			if !deleted.Diagnostics.HasError() || svc.deleted {
				t.Fatalf("delete diagnostics=%v deleted=%t", deleted.Diagnostics, svc.deleted)
			}
		})
	}
}

func TestVolumeSnapshotScheduleResourceReadNotFoundRemovesState(t *testing.T) {
	svc := &fakeVolumeSnapshotScheduleService{getErr: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := runRead(t, internalprovider.NewVolumeSnapshotScheduleResourceWithService(svc), volumeSnapshotScheduleConfig("policy-1", false))
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected state removal after scheduler policy is absent")
	}
}

func TestVolumeSnapshotScheduleResourceImportScopesPolicy(t *testing.T) {
	r := internalprovider.NewVolumeSnapshotScheduleResource().(resource.ResourceWithImportState)
	var schemaResponse resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
	_, nullState := rawFor(t, r, map[string]tftypes.Value{})
	resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: schemaResponse.Schema, Raw: nullState}}
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: "policy-1/yow-1/project-1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}
	var id, region, project string
	for name, target := range map[string]*string{"id": &id, "region": &region, "project": &project} {
		if diags := resp.State.GetAttribute(context.Background(), path.Root(name), target); diags.HasError() {
			t.Fatalf("%s: %v", name, diags)
		}
	}
	if id != "policy-1" || region != "yow-1" || project != "project-1" {
		t.Fatalf("imported %q/%q/%q", id, region, project)
	}
}
