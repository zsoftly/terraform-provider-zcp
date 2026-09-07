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
	"github.com/zsoftly/zcp-cli/pkg/api/firewall"
	"github.com/zsoftly/zcp-cli/pkg/api/ipaddress"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeFirewallService satisfies firewallServiceIface.
type fakeFirewallService struct {
	rules        []firewall.FirewallRule
	err          error
	deleted      []string
	createCalled bool
}

func (f *fakeFirewallService) List(_ context.Context, _ string) ([]firewall.FirewallRule, error) {
	return f.rules, f.err
}

// Create returns no rule object, matching the live API's asynchronous accept
// (data: null). The resource recovers the rule by polling List.
func (f *fakeFirewallService) Create(_ context.Context, _ string, _ firewall.CreateRequest) (*firewall.FirewallRule, error) {
	f.createCalled = true
	return nil, f.err
}
func (f *fakeFirewallService) Delete(_ context.Context, _ string, ruleID string) error {
	f.deleted = append(f.deleted, ruleID)
	return f.err
}

// fakeFirewallIPLister satisfies publicIPLister.
type fakeFirewallIPLister struct {
	ips             []ipaddress.IPAddress
	err             error
	lastListProject string
}

func (f *fakeFirewallIPLister) List(_ context.Context, _, _, project string) ([]ipaddress.IPAddress, error) {
	f.lastListProject = project
	return f.ips, f.err
}

// firewallRuleStateModel mirrors firewallRuleResourceModel for state extraction in tests.
type firewallRuleStateModel struct {
	ID                  types.String   `tfsdk:"id"`
	IPAddress           types.String   `tfsdk:"ip_address"`
	Protocol            types.String   `tfsdk:"protocol"`
	CIDRList            types.String   `tfsdk:"cidr_list"`
	DestinationCIDRList types.String   `tfsdk:"destination_cidr_list"`
	StartPort           types.String   `tfsdk:"start_port"`
	EndPort             types.String   `tfsdk:"end_port"`
	State               types.String   `tfsdk:"state"`
	Timeouts            timeouts.Value `tfsdk:"timeouts"`
}

func firewallRuleSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewFirewallRuleResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func firewallRuleTFType(t *testing.T) tftypes.Type {
	t.Helper()
	return firewallRuleSchema(t).Schema.Type().TerraformType(context.Background())
}

func createFirewallRule(t *testing.T, svc *fakeFirewallService, ipAddress, protocol, cidrList, startPort, endPort string) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewFirewallRuleResourceWithService(svc)
	return createFirewallRuleWithResource(t, r, ipAddress, protocol, cidrList, startPort, endPort)
}

// createFirewallRuleWithIPLister wires both the firewall service and a
// public-IP lister, exercising the VPC-public-IP guard in Create.
func createFirewallRuleWithIPLister(t *testing.T, svc *fakeFirewallService, ipSvc *fakeFirewallIPLister, ipAddress, protocol, cidrList, startPort, endPort string) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewFirewallRuleResourceWithServices(svc, ipSvc)
	return createFirewallRuleWithResource(t, r, ipAddress, protocol, cidrList, startPort, endPort)
}

// createFirewallRuleWithIPListerAndProject additionally wires a provider
// default project, exercising project-scoping of the VPC-public-IP guard's
// list call.
func createFirewallRuleWithIPListerAndProject(t *testing.T, svc *fakeFirewallService, ipSvc *fakeFirewallIPLister, defaultProject, ipAddress, protocol, cidrList, startPort, endPort string) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewFirewallRuleResourceWithServicesAndProject(svc, ipSvc, defaultProject)
	return createFirewallRuleWithResource(t, r, ipAddress, protocol, cidrList, startPort, endPort)
}

func createFirewallRuleWithResource(t *testing.T, r resource.Resource, ipAddress, protocol, cidrList, startPort, endPort string) resource.CreateResponse {
	t.Helper()
	schResp := firewallRuleSchema(t)
	tfType := firewallRuleTFType(t)
	planVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, nil),
		"ip_address":            tftypes.NewValue(tftypes.String, ipAddress),
		"protocol":              tftypes.NewValue(tftypes.String, protocol),
		"cidr_list":             tftypes.NewValue(tftypes.String, cidrList),
		"destination_cidr_list": tftypes.NewValue(tftypes.String, nil),
		"start_port":            tftypes.NewValue(tftypes.String, startPort),
		"end_port":              tftypes.NewValue(tftypes.String, endPort),
		"state":                 tftypes.NewValue(tftypes.String, nil),
		"timeouts":              timeoutsNull(t, schResp),
	})
	createReq := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: planVal},
	}
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)},
	}
	r.Create(context.Background(), createReq, createResp)
	return *createResp
}

func readFirewallRule(t *testing.T, svc *fakeFirewallService, ipAddress, id string) resource.ReadResponse {
	t.Helper()
	r := internalprovider.NewFirewallRuleResourceWithService(svc)
	schResp := firewallRuleSchema(t)
	tfType := firewallRuleTFType(t)
	stateVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, id),
		"ip_address":            tftypes.NewValue(tftypes.String, ipAddress),
		"protocol":              tftypes.NewValue(tftypes.String, "tcp"),
		"cidr_list":             tftypes.NewValue(tftypes.String, "0.0.0.0/0"),
		"destination_cidr_list": tftypes.NewValue(tftypes.String, nil),
		"start_port":            tftypes.NewValue(tftypes.String, "80"),
		"end_port":              tftypes.NewValue(tftypes.String, "80"),
		"state":                 tftypes.NewValue(tftypes.String, "Active"),
		"timeouts":              timeoutsNull(t, schResp),
	})
	readReq := resource.ReadRequest{
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	readResp := &resource.ReadResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	r.Read(context.Background(), readReq, readResp)
	return *readResp
}

func deleteFirewallRule(t *testing.T, svc *fakeFirewallService, ipAddress, id string) resource.DeleteResponse {
	t.Helper()
	r := internalprovider.NewFirewallRuleResourceWithService(svc)
	schResp := firewallRuleSchema(t)
	tfType := firewallRuleTFType(t)
	stateVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, id),
		"ip_address":            tftypes.NewValue(tftypes.String, ipAddress),
		"protocol":              tftypes.NewValue(tftypes.String, "tcp"),
		"cidr_list":             tftypes.NewValue(tftypes.String, "0.0.0.0/0"),
		"destination_cidr_list": tftypes.NewValue(tftypes.String, nil),
		"start_port":            tftypes.NewValue(tftypes.String, "80"),
		"end_port":              tftypes.NewValue(tftypes.String, "80"),
		"state":                 tftypes.NewValue(tftypes.String, "Active"),
		"timeouts":              timeoutsNull(t, schResp),
	})
	deleteReq := resource.DeleteRequest{
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

func TestFirewallRuleResource_createHappyPath(t *testing.T) {
	// Creation returns no rule object (data: null), so the resource recovers the
	// ID by polling the list and matching on protocol, ports, and CIDR.
	svc := &fakeFirewallService{
		rules: []firewall.FirewallRule{
			{
				ID:        "fw-uuid-1",
				Protocol:  "tcp",
				CIDRList:  "0.0.0.0/0",
				StartPort: "80",
				EndPort:   "80",
				State:     "Active",
			},
		},
	}
	resp := createFirewallRule(t, svc, "1036521143", "tcp", "0.0.0.0/0", "80", "80")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got firewallRuleStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "fw-uuid-1" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "fw-uuid-1")
	}
	if got.State.ValueString() != "Active" {
		t.Errorf("State = %q, want %q", got.State.ValueString(), "Active")
	}
	if got.IPAddress.ValueString() != "1036521143" {
		t.Errorf("IPAddress = %q, want %q", got.IPAddress.ValueString(), "1036521143")
	}
}

func TestFirewallRuleResource_createServiceError(t *testing.T) {
	svc := &fakeFirewallService{err: errors.New("quota exceeded")}
	resp := createFirewallRule(t, svc, "1036521143", "tcp", "0.0.0.0/0", "80", "80")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

// The live API returns ports as JSON numbers (float64), not strings, so the
// match must render them through fwPortString.
func TestFirewallRuleResource_createMatchesNumericPorts(t *testing.T) {
	svc := &fakeFirewallService{
		rules: []firewall.FirewallRule{
			{ID: "fw-num", Protocol: "tcp", CIDRList: "0.0.0.0/0", StartPort: float64(80), EndPort: float64(80), State: "Active"},
		},
	}
	resp := createFirewallRule(t, svc, "1036521143", "tcp", "0.0.0.0/0", "80", "80")
	if resp.Diagnostics.HasError() {
		t.Fatalf("numeric ports should match: %v", resp.Diagnostics)
	}
	var got firewallRuleStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "fw-num" {
		t.Errorf("ID = %q, want fw-num (matched via numeric port)", got.ID.ValueString())
	}
}

// The API may echo cidr_list in a different order and spacing. The match must
// still find the rule (regression for the exact-string-match bug).
func TestFirewallRuleResource_createMatchesReorderedCIDR(t *testing.T) {
	svc := &fakeFirewallService{
		rules: []firewall.FirewallRule{
			{ID: "fw-cidr", Protocol: "tcp", CIDRList: "192.168.0.0/16, 10.0.0.0/8", StartPort: "80", EndPort: "80", State: "Active"},
		},
	}
	resp := createFirewallRule(t, svc, "1036521143", "tcp", "10.0.0.0/8,192.168.0.0/16", "80", "80")
	if resp.Diagnostics.HasError() {
		t.Fatalf("reordered cidr_list should still match: %v", resp.Diagnostics)
	}
	var got firewallRuleStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "fw-cidr" {
		t.Errorf("ID = %q, want fw-cidr (matched via unordered CIDR set)", got.ID.ValueString())
	}
}

// When no listed rule matches, Create must surface the "did not appear" error
// rather than hang. A short create timeout exercises the poll-timeout branch.
func TestFirewallRuleResource_createNeverAppears(t *testing.T) {
	svc := &fakeFirewallService{
		rules: []firewall.FirewallRule{
			{ID: "other", Protocol: "udp", StartPort: "53", EndPort: "53"},
		},
	}
	r := internalprovider.NewFirewallRuleResourceWithService(svc)
	schResp := firewallRuleSchema(t)
	tfType := firewallRuleTFType(t)
	planVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, nil),
		"ip_address":            tftypes.NewValue(tftypes.String, "1036521143"),
		"protocol":              tftypes.NewValue(tftypes.String, "tcp"),
		"cidr_list":             tftypes.NewValue(tftypes.String, "0.0.0.0/0"),
		"destination_cidr_list": tftypes.NewValue(tftypes.String, nil),
		"start_port":            tftypes.NewValue(tftypes.String, "80"),
		"end_port":              tftypes.NewValue(tftypes.String, "80"),
		"state":                 tftypes.NewValue(tftypes.String, nil),
		"timeouts":              timeoutsNull(t, schResp),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	createReq := resource.CreateRequest{Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: planVal}}
	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)}}
	r.Create(ctx, createReq, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected 'did not appear after create' error when no rule matches")
	}
}

// A matched rule returned without an ID must not be persisted, or the resource
// falls back into the recreate loop. Create must error instead.
func TestFirewallRuleResource_createEmptyIDErrors(t *testing.T) {
	svc := &fakeFirewallService{
		rules: []firewall.FirewallRule{
			{ID: "", Protocol: "tcp", CIDRList: "0.0.0.0/0", StartPort: "80", EndPort: "80", State: "Active"},
		},
	}
	resp := createFirewallRule(t, svc, "1036521143", "tcp", "0.0.0.0/0", "80", "80")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when the matched rule has an empty ID")
	}
}

// A public IP that belongs to a VPC (non-empty VPCID) must be rejected before
// Create calls the firewall service: the API accepts the request but never
// applies it, so a create would otherwise poll until timeout. The firewall
// service's Create must never be invoked in that case.
func TestFirewallRuleResource_createRejectsVPCPublicIP(t *testing.T) {
	svc := &fakeFirewallService{}
	ipSvc := &fakeFirewallIPLister{
		ips: []ipaddress.IPAddress{
			{Slug: "1036521143", VPCID: "vpc-1"},
		},
	}
	resp := createFirewallRuleWithIPLister(t, svc, ipSvc, "1036521143", "tcp", "0.0.0.0/0", "80", "80")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a firewall rule on a VPC public IP")
	}
	if svc.createCalled {
		t.Error("svc.Create must not be called when the guard rejects a VPC public IP")
	}
}

// The guard has no region/project attributes of its own to scope with, so it
// falls back to the provider's default project when one is configured.
func TestFirewallRuleResource_createGuardScopesListToDefaultProject(t *testing.T) {
	svc := &fakeFirewallService{
		rules: []firewall.FirewallRule{
			{ID: "fw-net", Protocol: "tcp", CIDRList: "0.0.0.0/0", StartPort: "80", EndPort: "80", State: "Active"},
		},
	}
	ipSvc := &fakeFirewallIPLister{ips: []ipaddress.IPAddress{{Slug: "1036521143", NetworkID: "net-1"}}}
	resp := createFirewallRuleWithIPListerAndProject(t, svc, ipSvc, "prod", "1036521143", "tcp", "0.0.0.0/0", "80", "80")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if ipSvc.lastListProject != "prod" {
		t.Errorf("List called with project %q, want %q", ipSvc.lastListProject, "prod")
	}
}

// With no default project configured, the guard's list call stays unscoped.
func TestFirewallRuleResource_createGuardListUnscopedWithoutDefaultProject(t *testing.T) {
	svc := &fakeFirewallService{
		rules: []firewall.FirewallRule{
			{ID: "fw-net", Protocol: "tcp", CIDRList: "0.0.0.0/0", StartPort: "80", EndPort: "80", State: "Active"},
		},
	}
	ipSvc := &fakeFirewallIPLister{ips: []ipaddress.IPAddress{{Slug: "1036521143", NetworkID: "net-1"}}}
	resp := createFirewallRuleWithIPLister(t, svc, ipSvc, "1036521143", "tcp", "0.0.0.0/0", "80", "80")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if ipSvc.lastListProject != "" {
		t.Errorf("List called with project %q, want empty (no default project configured)", ipSvc.lastListProject)
	}
}

// An isolated-network public IP (empty VPCID) must proceed to Create
// normally.
func TestFirewallRuleResource_createAllowsIsolatedNetworkIP(t *testing.T) {
	svc := &fakeFirewallService{
		rules: []firewall.FirewallRule{
			{ID: "fw-net", Protocol: "tcp", CIDRList: "0.0.0.0/0", StartPort: "80", EndPort: "80", State: "Active"},
		},
	}
	ipSvc := &fakeFirewallIPLister{
		ips: []ipaddress.IPAddress{
			{Slug: "1036521143", NetworkID: "net-1"},
		},
	}
	resp := createFirewallRuleWithIPLister(t, svc, ipSvc, "1036521143", "tcp", "0.0.0.0/0", "80", "80")
	if resp.Diagnostics.HasError() {
		t.Fatalf("isolated-network IP should proceed to create: %v", resp.Diagnostics)
	}
	var got firewallRuleStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "fw-net" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "fw-net")
	}
}

// A lister failure must not block Create: the create call's own error
// handling already covers a genuinely invalid IP.
func TestFirewallRuleResource_createProceedsWhenIPListerErrors(t *testing.T) {
	svc := &fakeFirewallService{
		rules: []firewall.FirewallRule{
			{ID: "fw-net", Protocol: "tcp", CIDRList: "0.0.0.0/0", StartPort: "80", EndPort: "80", State: "Active"},
		},
	}
	ipSvc := &fakeFirewallIPLister{err: errors.New("boom")}
	resp := createFirewallRuleWithIPLister(t, svc, ipSvc, "1036521143", "tcp", "0.0.0.0/0", "80", "80")
	if resp.Diagnostics.HasError() {
		t.Fatalf("a lister error should not block create: %v", resp.Diagnostics)
	}
}

// A slug not found in the account IP list must not block Create.
func TestFirewallRuleResource_createProceedsWhenIPNotFound(t *testing.T) {
	svc := &fakeFirewallService{
		rules: []firewall.FirewallRule{
			{ID: "fw-net", Protocol: "tcp", CIDRList: "0.0.0.0/0", StartPort: "80", EndPort: "80", State: "Active"},
		},
	}
	ipSvc := &fakeFirewallIPLister{ips: []ipaddress.IPAddress{{Slug: "other-slug"}}}
	resp := createFirewallRuleWithIPLister(t, svc, ipSvc, "1036521143", "tcp", "0.0.0.0/0", "80", "80")
	if resp.Diagnostics.HasError() {
		t.Fatalf("an unlisted IP should not block create: %v", resp.Diagnostics)
	}
}

func TestFirewallRuleResource_readFound(t *testing.T) {
	svc := &fakeFirewallService{
		rules: []firewall.FirewallRule{
			{
				ID:        "fw-uuid-1",
				Protocol:  "tcp",
				CIDRList:  "0.0.0.0/0",
				StartPort: "80",
				EndPort:   "80",
				State:     "Active",
			},
		},
	}
	resp := readFirewallRule(t, svc, "1036521143", "fw-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got firewallRuleStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Protocol.ValueString() != "tcp" {
		t.Errorf("Protocol = %q, want %q", got.Protocol.ValueString(), "tcp")
	}
	if got.StartPort.ValueString() != "80" {
		t.Errorf("StartPort = %q, want %q", got.StartPort.ValueString(), "80")
	}
	if got.EndPort.ValueString() != "80" {
		t.Errorf("EndPort = %q, want %q", got.EndPort.ValueString(), "80")
	}
	if got.CIDRList.ValueString() != "0.0.0.0/0" {
		t.Errorf("CIDRList = %q, want %q", got.CIDRList.ValueString(), "0.0.0.0/0")
	}
}

func TestFirewallRuleResource_readNotFound(t *testing.T) {
	svc := &fakeFirewallService{rules: []firewall.FirewallRule{}}
	resp := readFirewallRule(t, svc, "1036521143", "fw-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("read-not-found should not produce diagnostics: %v", resp.Diagnostics)
	}
	// RemoveResource sets the state to null.
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestFirewallRuleResource_deleteHappyPath(t *testing.T) {
	svc := &fakeFirewallService{}
	resp := deleteFirewallRule(t, svc, "1036521143", "fw-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "fw-uuid-1" {
		t.Errorf("Delete called with %v, want [fw-uuid-1]", svc.deleted)
	}
}

func TestFirewallRuleResource_delete404IsNoOp(t *testing.T) {
	svc := &fakeFirewallService{err: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := deleteFirewallRule(t, svc, "1036521143", "fw-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 on delete should be a no-op: %v", resp.Diagnostics)
	}
}
