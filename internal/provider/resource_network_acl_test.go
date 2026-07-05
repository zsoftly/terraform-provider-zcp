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

// fakeACLService satisfies aclServiceIface.
type fakeACLService struct {
	acls        []acl.NetworkACL
	rules       []acl.Rule
	createErr   error
	createdReq  acl.ACLCreateRequest
	deletedACLs []string
	replaced    []string // "<network>-><aclID>"
	ruleReq     acl.RuleCreateRequest
	ruleUpdReq  acl.RuleCreateRequest
	deletedRule []string
}

func (f *fakeACLService) List(_ context.Context, _ string) ([]acl.NetworkACL, error) {
	return f.acls, nil
}
func (f *fakeACLService) Create(_ context.Context, _ string, req acl.ACLCreateRequest) error {
	f.createdReq = req
	return f.createErr
}
func (f *fakeACLService) Delete(_ context.Context, _, aclID string) error {
	f.deletedACLs = append(f.deletedACLs, aclID)
	return nil
}
func (f *fakeACLService) ListRules(_ context.Context, _, _ string) ([]acl.Rule, error) {
	return f.rules, nil
}
func (f *fakeACLService) CreateRule(_ context.Context, _, _ string, req acl.RuleCreateRequest) error {
	f.ruleReq = req
	return nil
}
func (f *fakeACLService) UpdateRule(_ context.Context, _, _, _ string, req acl.RuleCreateRequest) error {
	f.ruleUpdReq = req
	return nil
}
func (f *fakeACLService) DeleteRule(_ context.Context, _, _, ruleID string) error {
	f.deletedRule = append(f.deletedRule, ruleID)
	return nil
}
func (f *fakeACLService) ReplaceNetworkACL(_ context.Context, networkSlug, aclID string) error {
	f.replaced = append(f.replaced, networkSlug+"->"+aclID)
	return nil
}

func aclSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewNetworkACLResource()
	var s resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &s)
	return s
}

func TestNetworkACLResource_createResolvesIDByName(t *testing.T) {
	svc := &fakeACLService{acls: []acl.NetworkACL{{ID: "acl-uuid-1", Name: "web-acl"}}}
	r := internalprovider.NewNetworkACLResourceWithService(svc)
	schResp := aclSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	planVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, nil),
		"name":        tftypes.NewValue(tftypes.String, "web-acl"),
		"vpc":         tftypes.NewValue(tftypes.String, "main-vpc"),
		"description": tftypes.NewValue(tftypes.String, nil),
	})
	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)}}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: planVal}}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", createResp.Diagnostics)
	}
	if svc.createdReq.VPC != "main-vpc" || svc.createdReq.Name != "web-acl" {
		t.Errorf("create request = %+v", svc.createdReq)
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
	if got.ID.ValueString() != "acl-uuid-1" {
		t.Errorf("resolved ID = %q, want acl-uuid-1", got.ID.ValueString())
	}
}

func TestNetworkACLResource_createNotFoundAfterCreate(t *testing.T) {
	svc := &fakeACLService{acls: []acl.NetworkACL{}} // List returns nothing
	r := internalprovider.NewNetworkACLResourceWithService(svc)
	schResp := aclSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	planVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, nil),
		"name":        tftypes.NewValue(tftypes.String, "web-acl"),
		"vpc":         tftypes.NewValue(tftypes.String, "main-vpc"),
		"description": tftypes.NewValue(tftypes.String, nil),
	})
	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)}}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: planVal}}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error when created ACL cannot be resolved")
	}
}
