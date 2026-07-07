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
	"github.com/zsoftly/zcp-cli/pkg/api/egress"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeEgressService satisfies egressServiceIface.
type fakeEgressService struct {
	rules     []egress.EgressRule
	created   *egress.EgressRule
	createReq egress.CreateRequest
	err       error
	deleted   []string
}

func (f *fakeEgressService) List(_ context.Context, _ string) ([]egress.EgressRule, error) {
	return f.rules, f.err
}
func (f *fakeEgressService) Create(_ context.Context, req egress.CreateRequest) (*egress.EgressRule, error) {
	f.createReq = req
	return f.created, f.err
}
func (f *fakeEgressService) Delete(_ context.Context, _ string, ruleID string) error {
	f.deleted = append(f.deleted, ruleID)
	return f.err
}

// egressRuleStateModel mirrors egressRuleResourceModel for state extraction in tests.
type egressRuleStateModel struct {
	ID        types.String   `tfsdk:"id"`
	Network   types.String   `tfsdk:"network"`
	Protocol  types.String   `tfsdk:"protocol"`
	CIDR      types.String   `tfsdk:"cidr"`
	StartPort types.String   `tfsdk:"start_port"`
	EndPort   types.String   `tfsdk:"end_port"`
	ICMPType  types.String   `tfsdk:"icmp_type"`
	ICMPCode  types.String   `tfsdk:"icmp_code"`
	State     types.String   `tfsdk:"state"`
	Timeouts  timeouts.Value `tfsdk:"timeouts"`
}

func egressRuleSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewEgressRuleResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func egressRuleTFType(t *testing.T) tftypes.Type {
	t.Helper()
	return egressRuleSchema(t).Schema.Type().TerraformType(context.Background())
}

func egressRuleRaw(t *testing.T, schResp resource.SchemaResponse, id, network, protocol, cidr, startPort, endPort, state string) tftypes.Value {
	t.Helper()
	str := func(v string) tftypes.Value {
		if v == "" {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, v)
	}
	return tftypes.NewValue(egressRuleTFType(t), map[string]tftypes.Value{
		"id":         str(id),
		"network":    str(network),
		"protocol":   str(protocol),
		"cidr":       str(cidr),
		"start_port": str(startPort),
		"end_port":   str(endPort),
		"icmp_type":  tftypes.NewValue(tftypes.String, nil),
		"icmp_code":  tftypes.NewValue(tftypes.String, nil),
		"state":      str(state),
		"timeouts":   timeoutsNull(t, schResp),
	})
}

func createEgressRule(t *testing.T, svc *fakeEgressService, network, protocol, cidr, startPort, endPort string) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewEgressRuleResourceWithService(svc)
	schResp := egressRuleSchema(t)
	planVal := egressRuleRaw(t, schResp, "", network, protocol, cidr, startPort, endPort, "")
	createReq := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: planVal},
	}
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(egressRuleTFType(t), nil)},
	}
	r.Create(context.Background(), createReq, createResp)
	return *createResp
}

func readEgressRule(t *testing.T, svc *fakeEgressService, network, id string) resource.ReadResponse {
	t.Helper()
	r := internalprovider.NewEgressRuleResourceWithService(svc)
	schResp := egressRuleSchema(t)
	stateVal := egressRuleRaw(t, schResp, id, network, "tcp", "0.0.0.0/0", "443", "443", "Active")
	readReq := resource.ReadRequest{
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	readResp := &resource.ReadResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	r.Read(context.Background(), readReq, readResp)
	return *readResp
}

func deleteEgressRule(t *testing.T, svc *fakeEgressService, network, id string) resource.DeleteResponse {
	t.Helper()
	r := internalprovider.NewEgressRuleResourceWithService(svc)
	schResp := egressRuleSchema(t)
	stateVal := egressRuleRaw(t, schResp, id, network, "tcp", "0.0.0.0/0", "443", "443", "Active")
	deleteReq := resource.DeleteRequest{
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

func TestEgressRuleResource_createHappyPath(t *testing.T) {
	svc := &fakeEgressService{
		created: &egress.EgressRule{
			ID:        "egress-uuid-1",
			Protocol:  "tcp",
			DestCIDR:  "0.0.0.0/0",
			StartPort: "443",
			EndPort:   "443",
			Status:    "Active",
		},
	}
	resp := createEgressRule(t, svc, "prod-net", "tcp", "0.0.0.0/0", "443", "443")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got egressRuleStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "egress-uuid-1" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "egress-uuid-1")
	}
	if got.State.ValueString() != "Active" {
		t.Errorf("State = %q, want %q", got.State.ValueString(), "Active")
	}
	if got.Network.ValueString() != "prod-net" {
		t.Errorf("Network = %q, want %q", got.Network.ValueString(), "prod-net")
	}
}

func TestEgressRuleResource_createServiceError(t *testing.T) {
	svc := &fakeEgressService{err: errors.New("quota exceeded")}
	resp := createEgressRule(t, svc, "prod-net", "tcp", "0.0.0.0/0", "443", "443")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func TestEgressRuleResource_readFound(t *testing.T) {
	svc := &fakeEgressService{
		rules: []egress.EgressRule{
			{
				ID:        "egress-uuid-1",
				Protocol:  "tcp",
				DestCIDR:  "10.0.0.0/8",
				StartPort: "443",
				EndPort:   "443",
				Status:    "Active",
			},
		},
	}
	resp := readEgressRule(t, svc, "prod-net", "egress-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got egressRuleStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Protocol.ValueString() != "tcp" {
		t.Errorf("Protocol = %q, want %q", got.Protocol.ValueString(), "tcp")
	}
	// cidr must refresh from the API's destcidr_list field.
	if got.CIDR.ValueString() != "10.0.0.0/8" {
		t.Errorf("CIDR = %q, want %q", got.CIDR.ValueString(), "10.0.0.0/8")
	}
	if got.StartPort.ValueString() != "443" {
		t.Errorf("StartPort = %q, want %q", got.StartPort.ValueString(), "443")
	}
}

func TestEgressRuleResource_readNotFound(t *testing.T) {
	svc := &fakeEgressService{rules: []egress.EgressRule{}}
	resp := readEgressRule(t, svc, "prod-net", "egress-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("read-not-found should not produce diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestEgressRuleResource_deleteHappyPath(t *testing.T) {
	svc := &fakeEgressService{}
	resp := deleteEgressRule(t, svc, "prod-net", "egress-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "egress-uuid-1" {
		t.Errorf("Delete called with %v, want [egress-uuid-1]", svc.deleted)
	}
}

func TestEgressRuleResource_delete404IsNoOp(t *testing.T) {
	svc := &fakeEgressService{err: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := deleteEgressRule(t, svc, "prod-net", "egress-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 on delete should be a no-op: %v", resp.Diagnostics)
	}
}

func TestEgressRuleResource_createICMP(t *testing.T) {
	svc := &fakeEgressService{created: &egress.EgressRule{ID: "er-icmp-1", Status: "Active"}}
	r := internalprovider.NewEgressRuleResourceWithService(svc)
	schResp := egressRuleSchema(t)
	raw := egressRuleRaw(t, schResp, "", "prod-net-x1", "icmp", "0.0.0.0/0", "", "", "")
	vals := map[string]tftypes.Value{}
	if err := raw.As(&vals); err != nil {
		t.Fatalf("decomposing raw: %v", err)
	}
	vals["icmp_type"] = tftypes.NewValue(tftypes.String, "8")
	vals["icmp_code"] = tftypes.NewValue(tftypes.String, "0")
	tfType := egressRuleTFType(t)
	createReq := resource.CreateRequest{Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, vals)}}
	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)}}
	r.Create(context.Background(), createReq, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", createResp.Diagnostics)
	}
	if svc.createReq.ICMPType != "8" || svc.createReq.ICMPCode != "0" {
		t.Errorf("createReq ICMP = %q/%q, want 8/0", svc.createReq.ICMPType, svc.createReq.ICMPCode)
	}
	if svc.createReq.StartPort != "" || svc.createReq.EndPort != "" {
		t.Errorf("createReq ports = %q/%q, want empty for ICMP", svc.createReq.StartPort, svc.createReq.EndPort)
	}
}

func TestEgressRuleResource_validateConfigProtocol(t *testing.T) {
	r := internalprovider.NewEgressRuleResourceWithService(nil).(resource.ResourceWithValidateConfig)
	schResp := egressRuleSchema(t)

	run := func(protocol string) resource.ValidateConfigResponse {
		raw := egressRuleRaw(t, schResp, "", "prod-net-x1", protocol, "0.0.0.0/0", "", "", "")
		req := resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schResp.Schema, Raw: raw}}
		var resp resource.ValidateConfigResponse
		r.ValidateConfig(context.Background(), req, &resp)
		return resp
	}

	for _, ok := range []string{"tcp", "udp", "icmp", "all", "TCP"} {
		if resp := run(ok); resp.Diagnostics.HasError() {
			t.Errorf("protocol %q rejected: %v", ok, resp.Diagnostics)
		}
	}
	if resp := run("gre"); !resp.Diagnostics.HasError() {
		t.Error("protocol gre accepted, want error")
	}
}
