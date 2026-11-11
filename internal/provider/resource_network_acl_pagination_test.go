package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/acl"
	"github.com/zsoftly/zcp-cli/pkg/httpclient"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// pagedACLServer serves 12 ACLs for one VPC, 10 per page, newest first, so the
// two oldest ACLs are only on page 2.
func pagedACLServer(t *testing.T) *httptest.Server {
	t.Helper()
	const total, perPage = 12, 10
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vpcs/main-vpc/network-acl-list" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"Success"}`))
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		var data []map[string]string
		for i := (page - 1) * perPage; i < total && i < page*perPage; i++ {
			n := total - i
			data = append(data, map[string]string{"id": fmt.Sprintf("acl-%02d", n), "name": fmt.Sprintf("acl-name-%02d", n)})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "Success", "current_page": page, "last_page": 2, "total": total, "data": data,
		})
	}))
}

func TestNetworkACLResource_readKeepsACLOnSecondPage(t *testing.T) {
	server := pagedACLServer(t)
	defer server.Close()
	svc := acl.NewService(httpclient.New(httpclient.Options{BaseURL: server.URL, BearerToken: "test", MaxRetries: -1}))
	r := internalprovider.NewNetworkACLResourceWithService(svc)

	schResp := aclSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	stateVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "acl-01"),
		"name":        tftypes.NewValue(tftypes.String, "acl-name-01"),
		"vpc":         tftypes.NewValue(tftypes.String, "main-vpc"),
		"description": tftypes.NewValue(tftypes.String, nil),
	})
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics)
	}
	if readResp.State.Raw.IsNull() {
		t.Fatal("ACL on the second list page was removed from state")
	}
	var got struct {
		ID          types.String `tfsdk:"id"`
		Name        types.String `tfsdk:"name"`
		VPC         types.String `tfsdk:"vpc"`
		Description types.String `tfsdk:"description"`
	}
	if diags := readResp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "acl-01" || got.Name.ValueString() != "acl-name-01" {
		t.Errorf("state = %+v, want acl-01 / acl-name-01", got)
	}
}

func TestNetworkACLResource_createResolvesIDOnSecondPage(t *testing.T) {
	server := pagedACLServer(t)
	defer server.Close()
	svc := acl.NewService(httpclient.New(httpclient.Options{BaseURL: server.URL, BearerToken: "test", MaxRetries: -1}))
	r := internalprovider.NewNetworkACLResourceWithService(svc)

	schResp := aclSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	planVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, nil),
		"name":        tftypes.NewValue(tftypes.String, "acl-name-01"),
		"vpc":         tftypes.NewValue(tftypes.String, "main-vpc"),
		"description": tftypes.NewValue(tftypes.String, nil),
	})
	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)}}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: planVal}}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", createResp.Diagnostics)
	}
	var got struct {
		ID          types.String `tfsdk:"id"`
		Name        types.String `tfsdk:"name"`
		VPC         types.String `tfsdk:"vpc"`
		Description types.String `tfsdk:"description"`
	}
	if diags := createResp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "acl-01" {
		t.Errorf("resolved ID = %q, want acl-01", got.ID.ValueString())
	}
}
