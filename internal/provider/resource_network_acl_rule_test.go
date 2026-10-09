package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/acl"
	"github.com/zsoftly/zcp-cli/pkg/httpclient"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

func aclRuleSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewNetworkACLRuleResource()
	var s resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &s)
	return s
}

type aclRuleState struct {
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

func aclRuleResourceWithHTTPService(t *testing.T, handler http.HandlerFunc) resource.Resource {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := httpclient.New(httpclient.Options{
		BaseURL:     server.URL,
		BearerToken: "test-token",
		Timeout:     5 * time.Second,
	})
	return internalprovider.NewNetworkACLRuleResourceWithService(acl.NewService(client))
}

func aclRulePage(t *testing.T, w http.ResponseWriter, page, total int, rules []acl.Rule) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"status":       "success",
		"current_page": page,
		"total":        total,
		"data":         rules,
	}); err != nil {
		t.Errorf("encoding ACL rule response: %v", err)
	}
}

func aclRuleSet(first, last int) []acl.Rule {
	rules := make([]acl.Rule, 0, last-first+1)
	for number := first; number <= last; number++ {
		rules = append(rules, acl.Rule{
			ID:          "rule-" + strconv.Itoa(number),
			Number:      number,
			Protocol:    "TCP",
			Action:      "Allow",
			TrafficType: "Ingress",
			CIDRList:    "0.0.0.0/0",
			StartPort:   "443",
			EndPort:     "443",
			Description: "HTTPS",
		})
	}
	return rules
}

func aclRuleReadState(t *testing.T, schema resource.SchemaResponse, id string, number int64) tfsdk.State {
	t.Helper()
	typeValue := schema.Schema.Type().TerraformType(context.Background())
	values := ruleValues(number, "tcp", ptr(number), ptr(number))
	values["id"] = tftypes.NewValue(tftypes.String, id)
	values["description"] = tftypes.NewValue(tftypes.String, "HTTPS")
	return tfsdk.State{Schema: schema.Schema, Raw: tftypes.NewValue(typeValue, values)}
}

func ptr(value int64) *int64 { return &value }

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

func TestNetworkACLRuleResource_readFindsRuleBeyondFirstPage(t *testing.T) {
	var requestedPages []string
	r := aclRuleResourceWithHTTPService(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", req.Method)
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		requestedPages = append(requestedPages, req.URL.Query().Get("page"))
		switch req.URL.Query().Get("page") {
		case "":
			aclRulePage(t, w, 1, 11, aclRuleSet(1, 10))
		case "2":
			aclRulePage(t, w, 2, 11, aclRuleSet(11, 11))
		default:
			t.Errorf("unexpected page %q", req.URL.Query().Get("page"))
			http.Error(w, "unexpected page", http.StatusBadRequest)
		}
	})
	schema := aclRuleSchema(t)
	state := aclRuleReadState(t, schema, "rule-11", 11)
	response := &resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, response)
	if response.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", response.Diagnostics)
	}
	if got, want := requestedPages, []string{"", "2"}; !slices.Equal(got, want) {
		t.Errorf("requested pages = %v, want %v", got, want)
	}
	var got aclRuleState
	if diags := response.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "rule-11" || got.Number.ValueInt64() != 11 {
		t.Errorf("state = id %q, number %d; want rule-11, 11", got.ID.ValueString(), got.Number.ValueInt64())
	}
}

func TestNetworkACLRuleResource_readLaterPageFailureRetainsState(t *testing.T) {
	r := aclRuleResourceWithHTTPService(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Query().Get("page") == "2" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{"))
			return
		}
		aclRulePage(t, w, 1, 11, aclRuleSet(1, 10))
	})
	schema := aclRuleSchema(t)
	state := aclRuleReadState(t, schema, "rule-11", 11)
	response := &resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, response)
	if !response.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for the malformed second page")
	}
	var got aclRuleState
	if diags := response.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading retained state: %v", diags)
	}
	if got.ID.ValueString() != "rule-11" {
		t.Errorf("retained ID = %q, want rule-11", got.ID.ValueString())
	}
}

func TestNetworkACLRuleResource_readRemovesOnlyAfterAllPages(t *testing.T) {
	var requestedPages []string
	r := aclRuleResourceWithHTTPService(t, func(w http.ResponseWriter, req *http.Request) {
		page := req.URL.Query().Get("page")
		requestedPages = append(requestedPages, page)
		if page == "2" {
			aclRulePage(t, w, 2, 11, aclRuleSet(12, 12))
			return
		}
		aclRulePage(t, w, 1, 11, aclRuleSet(1, 10))
	})
	schema := aclRuleSchema(t)
	state := aclRuleReadState(t, schema, "rule-11", 11)
	response := &resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, response)
	if response.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", response.Diagnostics)
	}
	if !response.State.Raw.IsNull() {
		t.Fatal("rule missing from every page must be removed from state")
	}
	if got, want := requestedPages, []string{"", "2"}; !slices.Equal(got, want) {
		t.Errorf("requested pages = %v, want %v", got, want)
	}
}

func TestNetworkACLRuleResource_createResolvesRuleBeyondFirstPage(t *testing.T) {
	var requestedPages []string
	r := aclRuleResourceWithHTTPService(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":{}}`))
		case http.MethodGet:
			requestedPages = append(requestedPages, req.URL.Query().Get("page"))
			if req.URL.Query().Get("page") == "2" {
				aclRulePage(t, w, 2, 11, aclRuleSet(100, 100))
				return
			}
			aclRulePage(t, w, 1, 11, aclRuleSet(1, 10))
		default:
			t.Errorf("method = %s, want POST or GET", req.Method)
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	})
	schema := aclRuleSchema(t)
	typeValue := schema.Schema.Type().TerraformType(context.Background())
	p443 := int64(443)
	plan := tftypes.NewValue(typeValue, ruleValues(100, "tcp", &p443, &p443))
	response := &resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema, Raw: tftypes.NewValue(typeValue, nil)}}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: schema.Schema, Raw: plan}}, response)
	if response.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", response.Diagnostics)
	}
	if got, want := requestedPages, []string{"", "2"}; !slices.Equal(got, want) {
		t.Errorf("requested pages = %v, want %v", got, want)
	}
	var got aclRuleState
	if diags := response.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "rule-100" {
		t.Errorf("resolved ID = %q, want rule-100", got.ID.ValueString())
	}
}

func TestNetworkACLRuleResource_createFailsWhenLaterPageCannotBeRead(t *testing.T) {
	r := aclRuleResourceWithHTTPService(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":{}}`))
		case http.MethodGet:
			if req.URL.Query().Get("page") == "2" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte("{"))
				return
			}
			aclRulePage(t, w, 1, 11, aclRuleSet(1, 10))
		default:
			t.Errorf("method = %s, want POST or GET", req.Method)
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	})
	schema := aclRuleSchema(t)
	typeValue := schema.Schema.Type().TerraformType(context.Background())
	p443 := int64(443)
	plan := tftypes.NewValue(typeValue, ruleValues(100, "tcp", &p443, &p443))
	response := &resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema, Raw: tftypes.NewValue(typeValue, nil)}}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: schema.Schema, Raw: plan}}, response)
	if !response.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic when resolving a created rule fails on page two")
	}
	if !response.State.Raw.IsNull() {
		t.Fatal("a failed create must not write partial state")
	}
}
