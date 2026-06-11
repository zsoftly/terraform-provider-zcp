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
	"github.com/zsoftly/zcp-cli/pkg/api/firewall"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeFirewallService satisfies firewallServiceIface.
type fakeFirewallService struct {
	rules   []firewall.FirewallRule
	created *firewall.FirewallRule
	err     error
	deleted []string
}

func (f *fakeFirewallService) List(_ context.Context, _ string) ([]firewall.FirewallRule, error) {
	return f.rules, f.err
}
func (f *fakeFirewallService) Create(_ context.Context, _ string, _ firewall.CreateRequest) (*firewall.FirewallRule, error) {
	return f.created, f.err
}
func (f *fakeFirewallService) Delete(_ context.Context, _ string, ruleID string) error {
	f.deleted = append(f.deleted, ruleID)
	return f.err
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
	svc := &fakeFirewallService{
		created: &firewall.FirewallRule{
			ID:        "fw-uuid-1",
			Protocol:  "tcp",
			CIDRList:  "0.0.0.0/0",
			StartPort: "80",
			EndPort:   "80",
			State:     "Active",
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
