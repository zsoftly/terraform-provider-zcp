package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/billing"
	"github.com/zsoftly/zcp-cli/pkg/api/instance"
	"github.com/zsoftly/zcp-cli/pkg/api/ipaddress"
	"github.com/zsoftly/zcp-cli/pkg/api/permission"
	"github.com/zsoftly/zcp-cli/pkg/api/project"
	"github.com/zsoftly/zcp-cli/pkg/api/role"
	"github.com/zsoftly/zcp-cli/pkg/api/subuser"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

func runUpdate(t *testing.T, r resource.Resource, planCfg, stateCfg map[string]tftypes.Value) resource.UpdateResponse {
	t.Helper()
	schResp, stateVal := rawFor(t, r, stateCfg)
	_, planVal := rawFor(t, r, planCfg)
	updateReq := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schResp.Schema, Raw: planVal},
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Update(context.Background(), updateReq, updateResp)
	return *updateResp
}

// --- zcp_project ---

type fakeProjectService struct {
	projects  []project.Project
	created   *project.Project
	updateReq *project.UpdateRequest
	err       error
	deleted   []string
}

func (f *fakeProjectService) List(_ context.Context) ([]project.Project, error) {
	return f.projects, f.err
}
func (f *fakeProjectService) Create(_ context.Context, _ project.CreateRequest) (*project.Project, error) {
	return f.created, f.err
}
func (f *fakeProjectService) Update(_ context.Context, _ string, req project.UpdateRequest) (*project.Project, error) {
	f.updateReq = &req
	return &project.Project{}, f.err
}
func (f *fakeProjectService) Delete(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	if f.err == nil {
		f.projects = nil
	}
	return f.err
}

func projectConfig(id, name string) map[string]tftypes.Value {
	cfg := map[string]tftypes.Value{"name": strVal(name)}
	if id != "" {
		cfg["id"] = strVal(id)
	}
	return cfg
}

type projectStateShim struct {
	ID          types.String   `tfsdk:"id"`
	Name        types.String   `tfsdk:"name"`
	Description types.String   `tfsdk:"description"`
	Purpose     types.String   `tfsdk:"purpose"`
	Icon        types.String   `tfsdk:"icon"`
	Timeouts    timeouts.Value `tfsdk:"timeouts"`
}

func TestProjectResource_createHappyPath(t *testing.T) {
	svc := &fakeProjectService{
		created: &project.Project{Slug: "staging-p1", Name: "staging"},
	}
	r := internalprovider.NewProjectResourceWithService(svc)
	resp := runCreate(t, r, projectConfig("", "staging"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := stateID(t, resp.State); got != "staging-p1" {
		t.Errorf("ID = %q, want staging-p1", got)
	}
}

func TestProjectResource_updateInPlace(t *testing.T) {
	svc := &fakeProjectService{}
	r := internalprovider.NewProjectResourceWithService(svc)
	resp := runUpdate(t, r, projectConfig("staging-p1", "staging-renamed"), projectConfig("staging-p1", "staging"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if svc.updateReq == nil || svc.updateReq.Name != "staging-renamed" {
		t.Errorf("updateReq = %+v, want name staging-renamed", svc.updateReq)
	}
}

func TestProjectResource_updatePreservesOptionalFieldsWhenUnset(t *testing.T) {
	svc := &fakeProjectService{}
	r := internalprovider.NewProjectResourceWithService(svc)
	stateCfg := projectConfig("staging-p1", "staging")
	stateCfg["description"] = strVal("old description")
	stateCfg["purpose"] = strVal("old purpose")
	planCfg := projectConfig("staging-p1", "staging-renamed")

	resp := runUpdate(t, r, planCfg, stateCfg)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got projectStateShim
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Description.ValueString() != "old description" {
		t.Errorf("Description = %q, want old description", got.Description.ValueString())
	}
	if got.Purpose.ValueString() != "old purpose" {
		t.Errorf("Purpose = %q, want old purpose", got.Purpose.ValueString())
	}
}

func TestProjectResource_readClearsEmptyOptionalFields(t *testing.T) {
	svc := &fakeProjectService{projects: []project.Project{{Slug: "staging-p1", Name: "staging"}}}
	r := internalprovider.NewProjectResourceWithService(svc)
	stateCfg := projectConfig("staging-p1", "staging")
	stateCfg["description"] = strVal("stale description")
	stateCfg["purpose"] = strVal("stale purpose")

	resp := runRead(t, r, stateCfg)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got projectStateShim
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if !got.Description.IsNull() {
		t.Errorf("Description = %q, want null", got.Description.ValueString())
	}
	if !got.Purpose.IsNull() {
		t.Errorf("Purpose = %q, want null", got.Purpose.ValueString())
	}
}

func TestProjectResource_deleteHappyPath(t *testing.T) {
	svc := &fakeProjectService{projects: []project.Project{{Slug: "staging-p1"}}}
	r := internalprovider.NewProjectResourceWithService(svc)
	resp := runDelete(t, r, projectConfig("staging-p1", "staging"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "staging-p1" {
		t.Errorf("Delete called with %v, want [staging-p1]", svc.deleted)
	}
}

// --- zcp_sub_user ---

type fakeSubUserService struct {
	users     []subuser.SubUser
	created   *subuser.SubUser
	createReq subuser.CreateRequest
	updateReq *subuser.UpdateRequest
	err       error
	deleted   []string
}

func (f *fakeSubUserService) List(_ context.Context) ([]subuser.SubUser, error) {
	return f.users, f.err
}
func (f *fakeSubUserService) Create(_ context.Context, req subuser.CreateRequest) (*subuser.SubUser, error) {
	f.createReq = req
	return f.created, f.err
}
func (f *fakeSubUserService) Update(_ context.Context, _ string, req subuser.UpdateRequest) (*subuser.SubUser, error) {
	f.updateReq = &req
	return &subuser.SubUser{}, f.err
}
func (f *fakeSubUserService) Delete(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	if f.err == nil {
		f.users = nil
	}
	return f.err
}

func subUserConfig(id string) map[string]tftypes.Value {
	cfg := map[string]tftypes.Value{
		"name":     strVal("Alice"),
		"email":    strVal("alice@example.com"),
		"password": strVal("Sup3r$ecret"),
		"role":     strVal("ops"),
		"projects": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{strVal("default-9")}),
	}
	if id != "" {
		cfg["id"] = strVal(id)
	}
	return cfg
}

func TestSubUserResource_createHappyPath(t *testing.T) {
	svc := &fakeSubUserService{
		created: &subuser.SubUser{ID: "u-1", Name: "Alice", Email: "alice@example.com", UserStatus: "active"},
	}
	r := internalprovider.NewSubUserResourceWithService(svc)
	resp := runCreate(t, r, subUserConfig(""))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := stateID(t, resp.State); got != "u-1" {
		t.Errorf("ID = %q, want u-1", got)
	}
	if svc.createReq.AuthUser != "customer" || !svc.createReq.IsUserPassword {
		t.Errorf("createReq = %+v, want AuthUser=customer IsUserPassword=true", svc.createReq)
	}
	if len(svc.createReq.Projects) != 1 || svc.createReq.Projects[0] != "default-9" {
		t.Errorf("createReq.Projects = %v, want [default-9]", svc.createReq.Projects)
	}
}

func TestSubUserResource_updateEchoesEmailAndProjects(t *testing.T) {
	svc := &fakeSubUserService{}
	r := internalprovider.NewSubUserResourceWithService(svc)
	planCfg := subUserConfig("u-1")
	planCfg["role"] = strVal("admin")
	resp := runUpdate(t, r, planCfg, subUserConfig("u-1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if svc.updateReq == nil {
		t.Fatal("Update not called")
	}
	if svc.updateReq.Role != "admin" {
		t.Errorf("updateReq.Role = %q, want admin", svc.updateReq.Role)
	}
	// The API rejects updates missing email/projects; both must be echoed.
	if svc.updateReq.Email != "alice@example.com" || len(svc.updateReq.Projects) != 1 {
		t.Errorf("updateReq = %+v, want email and projects echoed", svc.updateReq)
	}
}

func TestSubUserResource_deleteHappyPath(t *testing.T) {
	svc := &fakeSubUserService{users: []subuser.SubUser{{ID: "u-1"}}}
	r := internalprovider.NewSubUserResourceWithService(svc)
	resp := runDelete(t, r, subUserConfig("u-1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "u-1" {
		t.Errorf("Delete called with %v, want [u-1]", svc.deleted)
	}
}

// --- zcp_role ---

// roleStateShim mirrors roleResourceModel for state extraction in tests.
type roleStateShim struct {
	ID          types.String   `tfsdk:"id"`
	Name        types.String   `tfsdk:"name"`
	Description types.String   `tfsdk:"description"`
	Permissions types.Set      `tfsdk:"permissions"`
	Timeouts    timeouts.Value `tfsdk:"timeouts"`
}

type fakeRoleService struct {
	got       *role.Role
	created   *role.Role
	updateReq *role.UpdateRequest
	err       error
	deleted   []string
}

func (f *fakeRoleService) Get(_ context.Context, _ string) (*role.Role, error) {
	if f.got == nil {
		return nil, &apierrors.APIError{StatusCode: 404, Message: "not found"}
	}
	return f.got, f.err
}
func (f *fakeRoleService) Create(_ context.Context, _ role.CreateRequest) (*role.Role, error) {
	return f.created, f.err
}
func (f *fakeRoleService) Update(_ context.Context, _ string, req role.UpdateRequest) (*role.Role, error) {
	f.updateReq = &req
	return &role.Role{}, f.err
}
func (f *fakeRoleService) Delete(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	if f.err == nil {
		f.got = nil
	}
	return f.err
}

func roleConfig(id string, permissions ...string) map[string]tftypes.Value {
	permVals := make([]tftypes.Value, 0, len(permissions))
	for _, p := range permissions {
		permVals = append(permVals, strVal(p))
	}
	cfg := map[string]tftypes.Value{
		"name":        strVal("ops"),
		"permissions": tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, permVals),
	}
	if id != "" {
		cfg["id"] = strVal(id)
	}
	return cfg
}

func TestRoleResource_createHappyPath(t *testing.T) {
	svc := &fakeRoleService{
		created: &role.Role{Slug: "ops-r1", Name: "ops"},
	}
	r := internalprovider.NewRoleResourceWithService(svc)
	resp := runCreate(t, r, roleConfig("", "instances-view", "instances-manage"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := stateID(t, resp.State); got != "ops-r1" {
		t.Errorf("ID = %q, want ops-r1", got)
	}
}

func TestRoleResource_createRejectsEmptyPermissions(t *testing.T) {
	svc := &fakeRoleService{
		created: &role.Role{Slug: "ops-r1", Name: "ops"},
	}
	r := internalprovider.NewRoleResourceWithService(svc)
	resp := runCreate(t, r, roleConfig(""))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for empty permissions")
	}
}

func TestRoleResource_updateReplacesPermissions(t *testing.T) {
	svc := &fakeRoleService{}
	r := internalprovider.NewRoleResourceWithService(svc)
	resp := runUpdate(t, r,
		roleConfig("ops-r1", "instances-view"),
		roleConfig("ops-r1", "instances-view", "instances-manage"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if svc.updateReq == nil || len(svc.updateReq.Permissions) != 1 || svc.updateReq.Permissions[0] != "instances-view" {
		t.Errorf("updateReq = %+v, want permissions [instances-view]", svc.updateReq)
	}
}

func TestRoleResource_updateRejectsEmptyPermissions(t *testing.T) {
	svc := &fakeRoleService{}
	r := internalprovider.NewRoleResourceWithService(svc)
	resp := runUpdate(t, r,
		roleConfig("ops-r1"),
		roleConfig("ops-r1", "instances-view"))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for empty permissions")
	}
	if svc.updateReq != nil {
		t.Errorf("Update should not be called, got %+v", svc.updateReq)
	}
}

func TestRoleResource_readRefreshesPermissions(t *testing.T) {
	svc := &fakeRoleService{
		got: &role.Role{
			Slug: "ops-r1", Name: "ops",
			Permissions: []permission.Permission{{Slug: "instances-view"}},
		},
	}
	r := internalprovider.NewRoleResourceWithService(svc)
	resp := runRead(t, r, roleConfig("ops-r1", "instances-view", "stale-perm"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got struct {
		Permissions []string `tfsdk:"permissions"`
	}
	// Extract just the permissions attribute.
	var full roleStateShim
	if diags := resp.State.Get(context.Background(), &full); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if diags := full.Permissions.ElementsAs(context.Background(), &got.Permissions, false); diags.HasError() {
		t.Fatalf("reading permissions: %v", diags)
	}
	if len(got.Permissions) != 1 || got.Permissions[0] != "instances-view" {
		t.Errorf("permissions = %v, want [instances-view]", got.Permissions)
	}
}

func TestRoleResource_deleteHappyPath(t *testing.T) {
	svc := &fakeRoleService{got: &role.Role{Slug: "ops-r1"}}
	r := internalprovider.NewRoleResourceWithService(svc)
	resp := runDelete(t, r, roleConfig("ops-r1", "instances-view"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "ops-r1" {
		t.Errorf("Delete called with %v, want [ops-r1]", svc.deleted)
	}
}

// --- zcp_budget_alert ---

type fakeBudgetAlertService struct {
	settings billing.BudgetAlert
	setReqs  []billing.SetBudgetAlertRequest
	err      error
}

func (f *fakeBudgetAlertService) GetBudgetAlert(_ context.Context) (json.RawMessage, error) {
	if f.err != nil {
		return nil, f.err
	}
	raw, _ := json.Marshal(f.settings)
	return raw, nil
}
func (f *fakeBudgetAlertService) SetBudgetAlert(_ context.Context, req billing.SetBudgetAlertRequest) (json.RawMessage, error) {
	f.setReqs = append(f.setReqs, req)
	f.settings = billing.BudgetAlert{Amount: req.Amount, Threshold: req.Threshold, IsEnabled: req.IsEnabled}
	return nil, f.err
}

func budgetAlertConfig(id string, amount, threshold float64) map[string]tftypes.Value {
	cfg := map[string]tftypes.Value{
		"amount":    tftypes.NewValue(tftypes.Number, amount),
		"threshold": tftypes.NewValue(tftypes.Number, threshold),
	}
	if id != "" {
		cfg["id"] = strVal(id)
	}
	return cfg
}

func TestBudgetAlertResource_createSetsAlert(t *testing.T) {
	svc := &fakeBudgetAlertService{}
	r := internalprovider.NewBudgetAlertResourceWithService(svc)
	resp := runCreate(t, r, budgetAlertConfig("", 500, 80))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.setReqs) != 1 || svc.setReqs[0].Amount != 500 || !svc.setReqs[0].IsEnabled {
		t.Errorf("setReqs = %+v, want one enabled with amount 500", svc.setReqs)
	}
	if got := stateID(t, resp.State); got != "budget-alert" {
		t.Errorf("ID = %q, want budget-alert", got)
	}
}

func TestBudgetAlertResource_deleteDisables(t *testing.T) {
	svc := &fakeBudgetAlertService{}
	r := internalprovider.NewBudgetAlertResourceWithService(svc)
	resp := runDelete(t, r, budgetAlertConfig("budget-alert", 500, 80))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.setReqs) != 1 || svc.setReqs[0].IsEnabled {
		t.Errorf("setReqs = %+v, want one disable call", svc.setReqs)
	}
}

// --- zcp_remote_access_vpn ---

type fakeRemoteAccessVPNService struct {
	vpns     []ipaddress.RemoteAccessVPN
	enabled  *ipaddress.RemoteAccessVPN
	err      error
	disabled []string
}

func (f *fakeRemoteAccessVPNService) ListRemoteAccessVPNs(_ context.Context, _ string) ([]ipaddress.RemoteAccessVPN, error) {
	return f.vpns, f.err
}
func (f *fakeRemoteAccessVPNService) EnableRemoteAccessVPN(_ context.Context, _ string) (*ipaddress.RemoteAccessVPN, error) {
	return f.enabled, f.err
}
func (f *fakeRemoteAccessVPNService) DisableRemoteAccessVPN(_ context.Context, _, vpnID string) error {
	f.disabled = append(f.disabled, vpnID)
	if f.err == nil {
		f.vpns = nil
	}
	return f.err
}

func remoteAccessVPNConfig(id string) map[string]tftypes.Value {
	cfg := map[string]tftypes.Value{"ip_address": strVal("1036521143")}
	if id != "" {
		cfg["id"] = strVal(id)
	}
	return cfg
}

func TestRemoteAccessVPNResource_createHappyPath(t *testing.T) {
	svc := &fakeRemoteAccessVPNService{
		enabled: &ipaddress.RemoteAccessVPN{ID: "vpn-1", PublicIP: "203.0.113.10", State: "Running"},
	}
	r := internalprovider.NewRemoteAccessVPNResourceWithService(svc)
	resp := runCreate(t, r, remoteAccessVPNConfig(""))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := stateID(t, resp.State); got != "vpn-1" {
		t.Errorf("ID = %q, want vpn-1", got)
	}
}

func TestRemoteAccessVPNResource_createResolvesFromList(t *testing.T) {
	svc := &fakeRemoteAccessVPNService{
		enabled: &ipaddress.RemoteAccessVPN{},
		vpns:    []ipaddress.RemoteAccessVPN{{ID: "vpn-1", PublicIP: "203.0.113.10"}},
	}
	r := internalprovider.NewRemoteAccessVPNResourceWithService(svc)
	resp := runCreate(t, r, remoteAccessVPNConfig(""))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := stateID(t, resp.State); got != "vpn-1" {
		t.Errorf("ID = %q, want vpn-1", got)
	}
}

func TestRemoteAccessVPNResource_deleteDisables(t *testing.T) {
	svc := &fakeRemoteAccessVPNService{
		vpns: []ipaddress.RemoteAccessVPN{{ID: "vpn-1"}},
	}
	r := internalprovider.NewRemoteAccessVPNResourceWithService(svc)
	resp := runDelete(t, r, remoteAccessVPNConfig("vpn-1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.disabled) != 1 || svc.disabled[0] != "vpn-1" {
		t.Errorf("Disable called with %v, want [vpn-1]", svc.disabled)
	}
}

// --- zcp_instance / zcp_permissions data sources ---

type fakeInstanceGetter struct {
	vm  *instance.VirtualMachine
	err error
}

func (f *fakeInstanceGetter) Get(_ context.Context, _ string) (*instance.VirtualMachine, error) {
	return f.vm, f.err
}

func TestInstanceDataSource_found(t *testing.T) {
	getter := &fakeInstanceGetter{
		vm: &instance.VirtualMachine{Slug: "vm1-abc", Name: "vm1", State: "Running"},
	}
	resp := readDS(t, internalprovider.NewInstanceDataSourceWithGetter(getter), map[string]tftypes.Value{
		"slug": strVal("vm1-abc"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
}

func TestInstanceDataSource_notFound(t *testing.T) {
	getter := &fakeInstanceGetter{err: errors.New("not found")}
	resp := readDS(t, internalprovider.NewInstanceDataSourceWithGetter(getter), map[string]tftypes.Value{
		"slug": strVal("missing"),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected not-found error, got none")
	}
}

type fakePermissionLister struct {
	permissions []permission.Permission
}

func (f *fakePermissionLister) List(_ context.Context) ([]permission.Permission, error) {
	return f.permissions, nil
}

func TestPermissionsDataSource_filtersByCategory(t *testing.T) {
	lister := &fakePermissionLister{
		permissions: []permission.Permission{
			{Slug: "instances-view", Category: "compute"},
			{Slug: "billing-view", Category: "billing"},
		},
	}
	resp := readDS(t, internalprovider.NewPermissionsDataSourceWithLister(lister), map[string]tftypes.Value{
		"category": strVal("compute"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
}
