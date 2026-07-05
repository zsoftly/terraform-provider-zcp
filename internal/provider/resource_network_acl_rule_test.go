package provider_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/acl"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

func aclRuleSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewNetworkACLRuleResource()
	var s resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &s)
	return s
}

// ruleValues builds a full attribute map for an ACL rule.
func ruleValues(number int64, proto string, start, end *int64) map[string]tftypes.Value {
	nullStr := func() tftypes.Value { return tftypes.NewValue(tftypes.String, nil) }
	nullInt := func() tftypes.Value { return tftypes.NewValue(tftypes.Number, nil) }
	portVal := func(p *int64) tftypes.Value {
		if p == nil {
			return nullInt()
		}
		return tftypes.NewValue(tftypes.Number, *p)
	}
	return map[string]tftypes.Value{
		"id":           nullStr(),
		"vpc":          tftypes.NewValue(tftypes.String, "main-vpc"),
		"acl":          tftypes.NewValue(tftypes.String, "acl-1"),
		"number":       tftypes.NewValue(tftypes.Number, number),
		"action":       tftypes.NewValue(tftypes.String, "allow"),
		"traffic_type": tftypes.NewValue(tftypes.String, "ingress"),
		"protocol":     tftypes.NewValue(tftypes.String, proto),
		"cidr_list":    tftypes.NewValue(tftypes.String, "0.0.0.0/0"),
		"start_port":   portVal(start),
		"end_port":     portVal(end),
		"icmp_type":    nullInt(),
		"icmp_code":    nullInt(),
		"description":  nullStr(),
	}
}

func TestNetworkACLRuleResource_createResolvesIDByNumber(t *testing.T) {
	// The API returns the rule (with its UUID) only via ListRules; match by number.
	svc := &fakeACLService{rules: []acl.Rule{
		{ID: "rule-uuid-9", Number: 100, Protocol: "tcp", Action: "allow"},
	}}
	r := internalprovider.NewNetworkACLRuleResourceWithService(svc)
	schResp := aclRuleSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	p443 := int64(443)
	planVal := tftypes.NewValue(tfType, ruleValues(100, "tcp", &p443, &p443))
	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)}}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: planVal}}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", createResp.Diagnostics)
	}
	// Request built correctly (ports passed as pointers).
	if svc.ruleReq.Number != 100 || svc.ruleReq.Protocol != "tcp" || svc.ruleReq.CIDRList != "0.0.0.0/0" {
		t.Errorf("rule request = %+v", svc.ruleReq)
	}
	if svc.ruleReq.StartPort == nil || *svc.ruleReq.StartPort != 443 || svc.ruleReq.EndPort == nil || *svc.ruleReq.EndPort != 443 {
		t.Errorf("ports = %v/%v, want 443/443", svc.ruleReq.StartPort, svc.ruleReq.EndPort)
	}
	var got struct {
		ID          types.String `tfsdk:"id"`
		VPC         types.String `tfsdk:"vpc"`
		ACL         types.String `tfsdk:"acl"`
		Number      types.Int64  `tfsdk:"number"`
		Action      types.String `tfsdk:"action"`
		TrafficType types.String `tfsdk:"traffic_type"`
		Protocol    types.String `tfsdk:"protocol"`
		CIDRList    types.String `tfsdk:"cidr_list"`
		StartPort   types.Int64  `tfsdk:"start_port"`
		EndPort     types.Int64  `tfsdk:"end_port"`
		ICMPType    types.Int64  `tfsdk:"icmp_type"`
		ICMPCode    types.Int64  `tfsdk:"icmp_code"`
		Description types.String `tfsdk:"description"`
	}
	if diags := createResp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "rule-uuid-9" {
		t.Errorf("resolved rule ID = %q, want rule-uuid-9", got.ID.ValueString())
	}
}

func TestNetworkACLRuleResource_createIcmpNoPorts(t *testing.T) {
	svc := &fakeACLService{rules: []acl.Rule{{ID: "rule-icmp", Number: 50, Protocol: "icmp"}}}
	r := internalprovider.NewNetworkACLRuleResourceWithService(svc)
	schResp := aclRuleSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	planVal := tftypes.NewValue(tfType, ruleValues(50, "icmp", nil, nil))
	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)}}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: planVal}}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", createResp.Diagnostics)
	}
	if svc.ruleReq.StartPort != nil || svc.ruleReq.EndPort != nil {
		t.Errorf("icmp rule should send no ports, got %v/%v", svc.ruleReq.StartPort, svc.ruleReq.EndPort)
	}
}
