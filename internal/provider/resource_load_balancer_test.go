package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/ipaddress"
	"github.com/zsoftly/zcp-cli/pkg/api/loadbalancer"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeLBIPService satisfies lbIPServiceIface for the destroy IP-release path.
type fakeLBIPService struct {
	ips      []ipaddress.IPAddress
	released []string
}

func (f *fakeLBIPService) List(_ context.Context, _, _, _ string) ([]ipaddress.IPAddress, error) {
	return f.ips, nil
}

func (f *fakeLBIPService) Release(_ context.Context, slug string) error {
	f.released = append(f.released, slug)
	return nil
}

// fakeLoadBalancerService satisfies loadBalancerServiceIface.
type fakeLoadBalancerService struct {
	lbs         []loadbalancer.LoadBalancer
	created     *loadbalancer.LoadBalancer
	createReq   loadbalancer.CreateRequest
	err         error
	deleted     []string
	ruleReqs    []loadbalancer.CreateRuleRequest
	rulesGone   []string
	attachReqs  []loadbalancer.AttachVMRequest
	detachedVMs []string
	listRegion  string
	listProject string
}

func (f *fakeLoadBalancerService) List(_ context.Context, region, project string) ([]loadbalancer.LoadBalancer, error) {
	f.listRegion = region
	f.listProject = project
	return f.lbs, f.err
}
func (f *fakeLoadBalancerService) Create(_ context.Context, req loadbalancer.CreateRequest) (*loadbalancer.LoadBalancer, error) {
	f.createReq = req
	return f.created, f.err
}
func (f *fakeLoadBalancerService) Delete(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	if f.err == nil {
		f.lbs = nil // gone on the next List so pollUntilGone finishes
	}
	return f.err
}
func (f *fakeLoadBalancerService) CreateRule(_ context.Context, _ string, req loadbalancer.CreateRuleRequest) error {
	f.ruleReqs = append(f.ruleReqs, req)
	return f.err
}
func (f *fakeLoadBalancerService) DeleteRule(_ context.Context, _ string, ruleID string) error {
	f.rulesGone = append(f.rulesGone, ruleID)
	if f.err == nil {
		for i := range f.lbs {
			var kept []loadbalancer.Rule
			for _, rule := range f.lbs[i].Rules {
				if rule.ID != ruleID {
					kept = append(kept, rule)
				}
			}
			f.lbs[i].Rules = kept
		}
	}
	return f.err
}
func (f *fakeLoadBalancerService) AttachVM(_ context.Context, _, _ string, req loadbalancer.AttachVMRequest) error {
	f.attachReqs = append(f.attachReqs, req)
	return f.err
}
func (f *fakeLoadBalancerService) DetachVM(_ context.Context, _, _ string, vmSlug string) error {
	f.detachedVMs = append(f.detachedVMs, vmSlug)
	return f.err
}

// --- zcp_load_balancer ---

type lbStateModel struct {
	ID              types.String   `tfsdk:"id"`
	Name            types.String   `tfsdk:"name"`
	CloudProvider   types.String   `tfsdk:"cloud_provider"`
	Region          types.String   `tfsdk:"region"`
	Project         types.String   `tfsdk:"project"`
	Network         types.String   `tfsdk:"network"`
	Plan            types.String   `tfsdk:"plan"`
	BillingCycle    types.String   `tfsdk:"billing_cycle"`
	AcquireNewIP    types.Bool     `tfsdk:"acquire_new_ip"`
	IPAddress       types.String   `tfsdk:"ip_address"`
	RuleName        types.String   `tfsdk:"rule_name"`
	PublicPort      types.String   `tfsdk:"public_port"`
	PrivatePort     types.String   `tfsdk:"private_port"`
	Protocol        types.String   `tfsdk:"protocol"`
	Algorithm       types.String   `tfsdk:"algorithm"`
	StickyMethod    types.String   `tfsdk:"sticky_method"`
	EnableTLS       types.Bool     `tfsdk:"enable_tls"`
	EnableProxy     types.Bool     `tfsdk:"enable_proxy"`
	VirtualMachines types.List     `tfsdk:"virtual_machines"`
	State           types.String   `tfsdk:"state"`
	PublicIP        types.String   `tfsdk:"public_ip"`
	RuleID          types.String   `tfsdk:"rule_id"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
}

func lbSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewLoadBalancerResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func lbRaw(t *testing.T, schResp resource.SchemaResponse, id, name, ruleID string) tftypes.Value {
	t.Helper()
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	str := func(v string) tftypes.Value {
		if v == "" {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, v)
	}
	return tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               str(id),
		"name":             str(name),
		"cloud_provider":   str("zsoftly"),
		"region":           str("yow-1"),
		"project":          tftypes.NewValue(tftypes.String, nil),
		"network":          str("prod-net"),
		"plan":             tftypes.NewValue(tftypes.String, nil),
		"billing_cycle":    str("hourly"),
		"acquire_new_ip":   tftypes.NewValue(tftypes.Bool, nil),
		"ip_address":       tftypes.NewValue(tftypes.String, nil),
		"rule_name":        str("https"),
		"public_port":      str("443"),
		"private_port":     str("8443"),
		"protocol":         tftypes.NewValue(tftypes.String, nil),
		"algorithm":        str("roundrobin"),
		"sticky_method":    tftypes.NewValue(tftypes.String, nil),
		"enable_tls":       tftypes.NewValue(tftypes.Bool, nil),
		"enable_proxy":     tftypes.NewValue(tftypes.Bool, nil),
		"virtual_machines": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"state":            tftypes.NewValue(tftypes.String, nil),
		"public_ip":        tftypes.NewValue(tftypes.String, nil),
		"rule_id":          str(ruleID),
		"timeouts":         timeoutsNull(t, schResp),
	})
}

func createLB(t *testing.T, svc *fakeLoadBalancerService) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewLoadBalancerResourceWithService(svc)
	schResp := lbSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	createReq := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: lbRaw(t, schResp, "", "web-lb", "")},
	}
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)},
	}
	r.Create(context.Background(), createReq, createResp)
	return *createResp
}

func readLB(t *testing.T, svc *fakeLoadBalancerService, id string) resource.ReadResponse {
	t.Helper()
	r := internalprovider.NewLoadBalancerResourceWithService(svc)
	schResp := lbSchema(t)
	stateVal := lbRaw(t, schResp, id, "web-lb", "rule-1")
	readReq := resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), readReq, readResp)
	return *readResp
}

func deleteLB(t *testing.T, svc *fakeLoadBalancerService, id string) resource.DeleteResponse {
	t.Helper()
	r := internalprovider.NewLoadBalancerResourceWithService(svc)
	schResp := lbSchema(t)
	stateVal := lbRaw(t, schResp, id, "web-lb", "rule-1")
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

// deleteLBIP runs Delete with both the LB and IP services. boundIP sets ip_address in
// state (empty means the LB acquired its own IP).
func deleteLBIP(t *testing.T, svc *fakeLoadBalancerService, ipSvc *fakeLBIPService, boundIP string) resource.DeleteResponse {
	t.Helper()
	r := internalprovider.NewLoadBalancerResourceWithServices(svc, ipSvc)
	schResp := lbSchema(t)
	stateVal := lbRaw(t, schResp, "web-lb-a1b2", "web-lb", "rule-1")
	if boundIP != "" {
		v := stateVal.Copy()
		obj := map[string]tftypes.Value{}
		_ = v.As(&obj)
		obj["ip_address"] = tftypes.NewValue(tftypes.String, boundIP)
		stateVal = tftypes.NewValue(v.Type(), obj)
	}
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

// TestLoadBalancerResource_deleteReleasesAcquiredIP verifies destroy releases the IP the LB
// acquired itself (no bound ip_address) — otherwise it orphans a billable address.
func TestLoadBalancerResource_deleteReleasesAcquiredIP(t *testing.T) {
	svc := &fakeLoadBalancerService{
		lbs: []loadbalancer.LoadBalancer{
			{Slug: "web-lb-a1b2", IPAddress: &loadbalancer.IPAddress{Slug: "ip-1", IPAddress: "203.0.113.9"}},
		},
	}
	// An attached LB IP reports an empty strategy; the gate is "not SOURCE-NAT".
	ipSvc := &fakeLBIPService{ips: []ipaddress.IPAddress{{Slug: "ip-1", IPAddress: "203.0.113.9", Strategy: ""}}}
	resp := deleteLBIP(t, svc, ipSvc, "")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 {
		t.Errorf("LB not deleted: %v", svc.deleted)
	}
	if len(ipSvc.released) != 1 || ipSvc.released[0] != "ip-1" {
		t.Errorf("released = %v, want [ip-1] (the LB owned its acquired IP)", ipSvc.released)
	}
}

// TestLoadBalancerResource_deleteSkipsSourceNATIP verifies destroy never releases a network
// source-NAT IP (the network owns it; releasing it would break the network).
func TestLoadBalancerResource_deleteSkipsSourceNATIP(t *testing.T) {
	svc := &fakeLoadBalancerService{
		lbs: []loadbalancer.LoadBalancer{
			{Slug: "web-lb-a1b2", IPAddress: &loadbalancer.IPAddress{Slug: "ip-snat", IPAddress: "203.0.113.1"}},
		},
	}
	ipSvc := &fakeLBIPService{ips: []ipaddress.IPAddress{{Slug: "ip-snat", Strategy: "SOURCE-NAT"}}}
	resp := deleteLBIP(t, svc, ipSvc, "")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(ipSvc.released) != 0 {
		t.Errorf("released a SOURCE-NAT IP %v; the network owns it and it must never be released", ipSvc.released)
	}
}

// TestLoadBalancerResource_deleteKeepsBoundIP verifies destroy leaves a bound zcp_ip_address
// alone — that IP is owned by its own resource.
func TestLoadBalancerResource_deleteKeepsBoundIP(t *testing.T) {
	svc := &fakeLoadBalancerService{
		lbs: []loadbalancer.LoadBalancer{
			{Slug: "web-lb-a1b2", IPAddress: &loadbalancer.IPAddress{Slug: "ip-1"}},
		},
	}
	ipSvc := &fakeLBIPService{ips: []ipaddress.IPAddress{{Slug: "ip-1", Strategy: "STATIC"}}}
	resp := deleteLBIP(t, svc, ipSvc, "existing-ip-slug")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(ipSvc.released) != 0 {
		t.Errorf("released a bound IP %v; ip_address is owned by its own resource and must not be released", ipSvc.released)
	}
}

func TestLoadBalancerResource_createHappyPath(t *testing.T) {
	svc := &fakeLoadBalancerService{
		created: &loadbalancer.LoadBalancer{
			Slug:  "web-lb-a1b2",
			Name:  "web-lb",
			State: "Active",
			Rules: []loadbalancer.Rule{
				{ID: "rule-1", Name: "https", PublicPort: "443", PrivatePort: "8443"},
			},
			IPAddress: &loadbalancer.IPAddress{IPAddress: "203.0.113.50"},
		},
	}
	resp := createLB(t, svc)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got lbStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "web-lb-a1b2" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "web-lb-a1b2")
	}
	if got.RuleID.ValueString() != "rule-1" {
		t.Errorf("RuleID = %q, want %q", got.RuleID.ValueString(), "rule-1")
	}
	if got.PublicIP.ValueString() != "203.0.113.50" {
		t.Errorf("PublicIP = %q, want %q", got.PublicIP.ValueString(), "203.0.113.50")
	}
	// No ip_address in config → the request must default to acquiring a new IP.
	if !svc.createReq.AcquireNewIP {
		t.Error("createReq.AcquireNewIP = false, want true (default)")
	}
	if len(svc.createReq.Rules) != 1 || svc.createReq.Rules[0].Name != "https" {
		t.Errorf("createReq.Rules = %+v, want one rule named https", svc.createReq.Rules)
	}
}

func TestLoadBalancerResource_createResolvesRuleFromList(t *testing.T) {
	// Create response omits rules and IP; the resource must refresh via List.
	svc := &fakeLoadBalancerService{
		created: &loadbalancer.LoadBalancer{Slug: "web-lb-a1b2", Name: "web-lb"},
		lbs: []loadbalancer.LoadBalancer{
			{
				Slug:  "web-lb-a1b2",
				Name:  "web-lb",
				State: "Active",
				Rules: []loadbalancer.Rule{
					{ID: "rule-1", Name: "https", PublicPort: "443", PrivatePort: "8443"},
				},
				IPAddress: &loadbalancer.IPAddress{IPAddress: "203.0.113.50"},
			},
		},
	}
	resp := createLB(t, svc)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got lbStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.RuleID.ValueString() != "rule-1" {
		t.Errorf("RuleID = %q, want %q", got.RuleID.ValueString(), "rule-1")
	}
	if got.PublicIP.ValueString() != "203.0.113.50" {
		t.Errorf("PublicIP = %q, want %q", got.PublicIP.ValueString(), "203.0.113.50")
	}
}

func TestLoadBalancerResource_createServiceError(t *testing.T) {
	svc := &fakeLoadBalancerService{err: errors.New("no capacity")}
	resp := createLB(t, svc)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func TestLoadBalancerResource_readFound(t *testing.T) {
	svc := &fakeLoadBalancerService{
		lbs: []loadbalancer.LoadBalancer{
			{
				Slug:  "web-lb-a1b2",
				Name:  "web-lb-renamed",
				State: "Active",
				Rules: []loadbalancer.Rule{
					{ID: "rule-1", Name: "https", PublicPort: "443", PrivatePort: "8443"},
				},
				IPAddress: &loadbalancer.IPAddress{IPAddress: "203.0.113.50"},
			},
		},
	}
	resp := readLB(t, svc, "web-lb-a1b2")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got lbStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Name.ValueString() != "web-lb-renamed" {
		t.Errorf("Name = %q, want %q", got.Name.ValueString(), "web-lb-renamed")
	}
	if got.State.ValueString() != "Active" {
		t.Errorf("State = %q, want %q", got.State.ValueString(), "Active")
	}
}

func TestLoadBalancerResource_readNotFound(t *testing.T) {
	svc := &fakeLoadBalancerService{}
	resp := readLB(t, svc, "web-lb-a1b2")
	if resp.Diagnostics.HasError() {
		t.Fatalf("read-not-found should not produce diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestLoadBalancerResource_deleteHappyPath(t *testing.T) {
	svc := &fakeLoadBalancerService{
		lbs: []loadbalancer.LoadBalancer{{Slug: "web-lb-a1b2"}},
	}
	resp := deleteLB(t, svc, "web-lb-a1b2")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "web-lb-a1b2" {
		t.Errorf("Delete called with %v, want [web-lb-a1b2]", svc.deleted)
	}
}

func TestLoadBalancerResource_delete404IsNoOp(t *testing.T) {
	svc := &fakeLoadBalancerService{err: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := deleteLB(t, svc, "web-lb-a1b2")
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 on delete should be a no-op: %v", resp.Diagnostics)
	}
}

// --- zcp_load_balancer_rule ---

type lbRuleStateModel struct {
	ID           types.String   `tfsdk:"id"`
	LoadBalancer types.String   `tfsdk:"load_balancer"`
	Name         types.String   `tfsdk:"name"`
	PublicPort   types.String   `tfsdk:"public_port"`
	PrivatePort  types.String   `tfsdk:"private_port"`
	Protocol     types.String   `tfsdk:"protocol"`
	Algorithm    types.String   `tfsdk:"algorithm"`
	StickyMethod types.String   `tfsdk:"sticky_method"`
	EnableTLS    types.Bool     `tfsdk:"enable_tls"`
	EnableProxy  types.Bool     `tfsdk:"enable_proxy"`
	Timeouts     timeouts.Value `tfsdk:"timeouts"`
}

func lbRuleSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewLoadBalancerRuleResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func lbRuleRaw(t *testing.T, schResp resource.SchemaResponse, id, lb, name, publicPort, privatePort string) tftypes.Value {
	t.Helper()
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	str := func(v string) tftypes.Value {
		if v == "" {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, v)
	}
	return tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            str(id),
		"load_balancer": str(lb),
		"name":          str(name),
		"public_port":   str(publicPort),
		"private_port":  str(privatePort),
		"protocol":      tftypes.NewValue(tftypes.String, nil),
		"algorithm":     str("roundrobin"),
		"sticky_method": tftypes.NewValue(tftypes.String, nil),
		"enable_tls":    tftypes.NewValue(tftypes.Bool, nil),
		"enable_proxy":  tftypes.NewValue(tftypes.Bool, nil),
		"timeouts":      timeoutsNull(t, schResp),
	})
}

func createLBRule(t *testing.T, svc *fakeLoadBalancerService, lb, name, publicPort, privatePort string) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewLoadBalancerRuleResourceWithService(svc)
	schResp := lbRuleSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	createReq := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: lbRuleRaw(t, schResp, "", lb, name, publicPort, privatePort)},
	}
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)},
	}
	r.Create(context.Background(), createReq, createResp)
	return *createResp
}

func TestLoadBalancerRuleResource_createResolvesID(t *testing.T) {
	svc := &fakeLoadBalancerService{
		lbs: []loadbalancer.LoadBalancer{
			{
				Slug: "web-lb-a1b2",
				Rules: []loadbalancer.Rule{
					{ID: "rule-1", Name: "https", PublicPort: "443", PrivatePort: "8443"},
					{ID: "rule-2", Name: "http", PublicPort: "80", PrivatePort: "8080"},
				},
			},
		},
	}
	resp := createLBRule(t, svc, "web-lb-a1b2", "http", "80", "8080")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got lbRuleStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "rule-2" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "rule-2")
	}
	if len(svc.ruleReqs) != 1 || len(svc.ruleReqs[0].Rules) != 1 {
		t.Fatalf("CreateRule requests = %+v, want exactly one with one rule", svc.ruleReqs)
	}
}

func TestLoadBalancerRuleResource_createServiceError(t *testing.T) {
	svc := &fakeLoadBalancerService{err: errors.New("port in use")}
	resp := createLBRule(t, svc, "web-lb-a1b2", "http", "80", "8080")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func TestLoadBalancerRuleResource_readGoneRemoves(t *testing.T) {
	svc := &fakeLoadBalancerService{
		lbs: []loadbalancer.LoadBalancer{{Slug: "web-lb-a1b2"}}, // rule list empty
	}
	r := internalprovider.NewLoadBalancerRuleResourceWithService(svc)
	schResp := lbRuleSchema(t)
	stateVal := lbRuleRaw(t, schResp, "rule-2", "web-lb-a1b2", "http", "80", "8080")
	readReq := resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), readReq, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics)
	}
	if !readResp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestLoadBalancerRuleResource_deleteHappyPath(t *testing.T) {
	svc := &fakeLoadBalancerService{
		lbs: []loadbalancer.LoadBalancer{
			{
				Slug:  "web-lb-a1b2",
				Rules: []loadbalancer.Rule{{ID: "rule-2", Name: "http", PublicPort: "80", PrivatePort: "8080"}},
			},
		},
	}
	r := internalprovider.NewLoadBalancerRuleResourceWithService(svc)
	schResp := lbRuleSchema(t)
	stateVal := lbRuleRaw(t, schResp, "rule-2", "web-lb-a1b2", "http", "80", "8080")
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", deleteResp.Diagnostics)
	}
	if len(svc.rulesGone) != 1 || svc.rulesGone[0] != "rule-2" {
		t.Errorf("DeleteRule called with %v, want [rule-2]", svc.rulesGone)
	}
}

// --- zcp_load_balancer_attachment ---

func lbAttachmentSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewLoadBalancerAttachmentResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func lbAttachmentRaw(t *testing.T, schResp resource.SchemaResponse, id, lb, rule, vm string) tftypes.Value {
	t.Helper()
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	str := func(v string) tftypes.Value {
		if v == "" {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, v)
	}
	return tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              str(id),
		"load_balancer":   str(lb),
		"rule":            str(rule),
		"virtual_machine": str(vm),
		"cloud_provider":  str("zsoftly"),
		"region":          str("yow-1"),
		"project":         tftypes.NewValue(tftypes.String, nil),
		"timeouts":        timeoutsNull(t, schResp),
	})
}

func TestLoadBalancerAttachmentResource_createHappyPath(t *testing.T) {
	svc := &fakeLoadBalancerService{}
	r := internalprovider.NewLoadBalancerAttachmentResourceWithService(svc)
	schResp := lbAttachmentSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	createReq := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: lbAttachmentRaw(t, schResp, "", "web-lb-a1b2", "rule-1", "vm1-abc")},
	}
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)},
	}
	r.Create(context.Background(), createReq, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", createResp.Diagnostics)
	}
	if len(svc.attachReqs) != 1 || len(svc.attachReqs[0].VirtualMachines) != 1 || svc.attachReqs[0].VirtualMachines[0] != "vm1-abc" {
		t.Errorf("AttachVM requests = %+v, want one with [vm1-abc]", svc.attachReqs)
	}
	if svc.attachReqs[0].Region != "yow-1" {
		t.Errorf("AttachVM region = %q, want yow-1", svc.attachReqs[0].Region)
	}
}

func TestLoadBalancerAttachmentResource_readRuleGoneRemoves(t *testing.T) {
	svc := &fakeLoadBalancerService{
		lbs: []loadbalancer.LoadBalancer{{Slug: "web-lb-a1b2"}}, // rule gone
	}
	r := internalprovider.NewLoadBalancerAttachmentResourceWithService(svc)
	schResp := lbAttachmentSchema(t)
	stateVal := lbAttachmentRaw(t, schResp, "web-lb-a1b2/rule-1/vm1-abc", "web-lb-a1b2", "rule-1", "vm1-abc")
	readReq := resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), readReq, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics)
	}
	if !readResp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestLoadBalancerAttachmentResource_readUsesProjectScope(t *testing.T) {
	// Read must resolve the project the same way Create does; an unscoped
	// list could miss the LB and wrongly drop the attachment from state.
	svc := &fakeLoadBalancerService{
		lbs: []loadbalancer.LoadBalancer{
			{Slug: "web-lb-a1b2", Rules: []loadbalancer.Rule{{ID: "rule-1"}}},
		},
	}
	r := internalprovider.NewLoadBalancerAttachmentResourceWithServiceAndProject(svc, "default-9")
	schResp := lbAttachmentSchema(t)
	stateVal := lbAttachmentRaw(t, schResp, "web-lb-a1b2/rule-1/vm1-abc", "web-lb-a1b2", "rule-1", "vm1-abc")
	readReq := resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), readReq, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics)
	}
	if readResp.State.Raw.IsNull() {
		t.Fatal("attachment was dropped from state although LB and rule exist")
	}
	if svc.listProject != "default-9" {
		t.Errorf("List called with project %q, want %q (provider default)", svc.listProject, "default-9")
	}
	if svc.listRegion != "yow-1" {
		t.Errorf("List called with region %q, want yow-1 (from state)", svc.listRegion)
	}
}

func TestLoadBalancerAttachmentResource_deleteDetaches(t *testing.T) {
	svc := &fakeLoadBalancerService{}
	r := internalprovider.NewLoadBalancerAttachmentResourceWithService(svc)
	schResp := lbAttachmentSchema(t)
	stateVal := lbAttachmentRaw(t, schResp, "web-lb-a1b2/rule-1/vm1-abc", "web-lb-a1b2", "rule-1", "vm1-abc")
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", deleteResp.Diagnostics)
	}
	if len(svc.detachedVMs) != 1 || svc.detachedVMs[0] != "vm1-abc" {
		t.Errorf("DetachVM called with %v, want [vm1-abc]", svc.detachedVMs)
	}
}
