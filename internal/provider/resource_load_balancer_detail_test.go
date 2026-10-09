package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/loadbalancer"
	"github.com/zsoftly/zcp-cli/pkg/httpclient"
)

func TestLoadBalancerResourceReadUsesDetailEndpoint(t *testing.T) {
	status := http.StatusOK
	rules := []loadbalancer.Rule{{ID: "rule-1", Name: "https", PublicPort: "443", PrivatePort: "8443"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/load-balancers" {
			t.Error("resource read must not request the load balancer collection")
			http.Error(w, "collection endpoint must not be used", http.StatusInternalServerError)
			return
		}
		if req.Method != http.MethodGet || req.URL.Path != "/load-balancers/web-lb-a1b2" {
			http.NotFound(w, req)
			return
		}
		if status == http.StatusNotFound {
			http.Error(w, "not found", status)
			return
		}
		if status != http.StatusOK {
			http.Error(w, "unavailable", status)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"status": "Success",
			"data": loadbalancer.LoadBalancer{
				Slug: "web-lb-a1b2", Name: "web-lb", State: "Active",
				Rules: rules,
			},
		})
	}))
	defer server.Close()

	client := httpclient.New(httpclient.Options{BaseURL: server.URL, BearerToken: "test-token"})
	r := &loadBalancerResource{svc: &loadBalancerService{Service: loadbalancer.NewService(client), client: client}}
	var schemaResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	state := loadBalancerReadState(t, schemaResp, "web-lb-a1b2")

	t.Run("detail response includes rules", func(t *testing.T) {
		resp := resource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: state}}
		r.Read(context.Background(), resource.ReadRequest{State: tfsdk.State{Schema: schemaResp.Schema, Raw: state}}, &resp)
		if resp.Diagnostics.HasError() || resp.State.Raw.IsNull() {
			t.Fatalf("read diagnostics = %v, state = %#v", resp.Diagnostics, resp.State.Raw)
		}
		var model loadBalancerResourceModel
		if diags := resp.State.Get(context.Background(), &model); diags.HasError() {
			t.Fatalf("reading state: %v", diags)
		}
		if model.RuleID.ValueString() != "rule-1" {
			t.Errorf("rule ID = %q, want rule-1 from the detail response", model.RuleID.ValueString())
		}
	})

	t.Run("not found removes state", func(t *testing.T) {
		status = http.StatusNotFound
		resp := resource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: state}}
		r.Read(context.Background(), resource.ReadRequest{State: tfsdk.State{Schema: schemaResp.Schema, Raw: state}}, &resp)
		if resp.Diagnostics.HasError() || !resp.State.Raw.IsNull() {
			t.Fatalf("read diagnostics = %v, state = %#v", resp.Diagnostics, resp.State.Raw)
		}
	})

	t.Run("missing initial rule clears rule ID", func(t *testing.T) {
		status = http.StatusOK
		rules = []loadbalancer.Rule{{ID: "rule-2", Name: "https", PublicPort: "443", PrivatePort: "8443"}}
		missingState := loadBalancerReadStateWithRule(t, schemaResp, "web-lb-a1b2", "http", "stale-http-rule")
		resp := resource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: missingState}}
		r.Read(context.Background(), resource.ReadRequest{State: tfsdk.State{Schema: schemaResp.Schema, Raw: missingState}}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("read diagnostics = %v", resp.Diagnostics)
		}
		var model loadBalancerResourceModel
		if diags := resp.State.Get(context.Background(), &model); diags.HasError() {
			t.Fatalf("reading state: %v", diags)
		}
		if !model.RuleID.IsNull() {
			t.Errorf("rule ID = %q, want null after the initial rule is absent", model.RuleID.ValueString())
		}
	})

	t.Run("later error preserves state", func(t *testing.T) {
		status = http.StatusServiceUnavailable
		resp := resource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: state}}
		r.Read(context.Background(), resource.ReadRequest{State: tfsdk.State{Schema: schemaResp.Schema, Raw: state}}, &resp)
		if !resp.Diagnostics.HasError() || resp.State.Raw.IsNull() {
			t.Fatalf("read diagnostics = %v, state = %#v", resp.Diagnostics, resp.State.Raw)
		}
	})
}

func loadBalancerReadState(t *testing.T, schemaResp resource.SchemaResponse, id string) tftypes.Value {
	return loadBalancerReadStateWithRule(t, schemaResp, id, "https", "")
}

func loadBalancerReadStateWithRule(t *testing.T, schemaResp resource.SchemaResponse, id, ruleName, ruleID string) tftypes.Value {
	t.Helper()
	tfType := schemaResp.Schema.Type().TerraformType(context.Background())
	obj := tfType.(tftypes.Object)
	values := make(map[string]tftypes.Value, len(obj.AttributeTypes))
	for name, attrType := range obj.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}
	values["id"] = tftypes.NewValue(tftypes.String, id)
	values["name"] = tftypes.NewValue(tftypes.String, "web-lb")
	values["rule_name"] = tftypes.NewValue(tftypes.String, ruleName)
	if ruleID != "" {
		values["rule_id"] = tftypes.NewValue(tftypes.String, ruleID)
	}
	return tftypes.NewValue(tfType, values)
}
