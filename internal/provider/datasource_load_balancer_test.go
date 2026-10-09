package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/loadbalancer"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

type fakeLoadBalancerGetter struct {
	lb     *loadbalancer.LoadBalancer
	err    error
	called string
}

func (f *fakeLoadBalancerGetter) Get(_ context.Context, slug string) (*loadbalancer.LoadBalancer, error) {
	f.called = slug
	return f.lb, f.err
}

type loadBalancerDSStateModel struct {
	Slug          types.String `tfsdk:"slug"`
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	State         types.String `tfsdk:"state"`
	PublicIP      types.String `tfsdk:"public_ip"`
	Region        types.String `tfsdk:"region"`
	Project       types.String `tfsdk:"project"`
	CloudProvider types.String `tfsdk:"cloud_provider"`
	Rules         types.List   `tfsdk:"rules"`
}

func TestLoadBalancerDataSource_found(t *testing.T) {
	getter := &fakeLoadBalancerGetter{lb: &loadbalancer.LoadBalancer{
		Slug:          "web-lb-a1b2",
		Name:          "web-lb",
		State:         "Active",
		IPAddress:     &loadbalancer.IPAddress{IPAddress: "203.0.113.50"},
		Region:        &loadbalancer.Region{Slug: "yow-1"},
		Project:       &loadbalancer.Project{Slug: "project-a"},
		CloudProvider: &loadbalancer.CloudProvider{Slug: "zsoftly"},
		Rules: []loadbalancer.Rule{{
			ID: "rule-1", Name: "https", PublicPort: "443", PrivatePort: "8443",
			Protocol: "tcp", Algorithm: "roundrobin", StickyMethod: "LbCookie",
			EnableTLSProtocol: true, EnableProxyProtocol: true,
		}},
	}}
	resp := readDS(t, internalprovider.NewLoadBalancerDataSourceWithGetter(getter), map[string]tftypes.Value{
		"slug": strVal("web-lb-a1b2"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got loadBalancerDSStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if getter.called != "web-lb-a1b2" || got.ID.ValueString() != "web-lb-a1b2" || got.PublicIP.ValueString() != "203.0.113.50" {
		t.Errorf("getter = %q, id = %q, public IP = %q", getter.called, got.ID.ValueString(), got.PublicIP.ValueString())
	}
	if got.Rules.IsNull() || len(got.Rules.Elements()) != 1 {
		t.Fatalf("rules = %#v, want one computed rule", got.Rules)
	}
	rule := got.Rules.Elements()[0].(types.Object).Attributes()
	if rule["id"].(types.String).ValueString() != "rule-1" ||
		!rule["enable_tls"].(types.Bool).ValueBool() ||
		!rule["enable_proxy"].(types.Bool).ValueBool() {
		t.Errorf("rule = %#v, want rule-1 with enabled TLS and proxy", rule)
	}
}

func TestLoadBalancerDataSource_emptyRulesIsComputedEmptyList(t *testing.T) {
	getter := &fakeLoadBalancerGetter{lb: &loadbalancer.LoadBalancer{Slug: "web-lb-a1b2"}}
	resp := readDS(t, internalprovider.NewLoadBalancerDataSourceWithGetter(getter), map[string]tftypes.Value{"slug": strVal("web-lb-a1b2")})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got loadBalancerDSStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Rules.IsNull() || len(got.Rules.Elements()) != 0 {
		t.Errorf("rules = %#v, want a non-null empty list", got.Rules)
	}
}

func TestLoadBalancerDataSource_notFound(t *testing.T) {
	getter := &fakeLoadBalancerGetter{err: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := readDS(t, internalprovider.NewLoadBalancerDataSourceWithGetter(getter), map[string]tftypes.Value{"slug": strVal("missing")})
	if !resp.Diagnostics.HasError() || resp.Diagnostics.Errors()[0].Summary() != "Load balancer not found" {
		t.Fatalf("diagnostics = %v, want load balancer not found", resp.Diagnostics)
	}
}

func TestLoadBalancerDataSource_missing403IsNotFound(t *testing.T) {
	getter := &fakeLoadBalancerGetter{err: &apierrors.APIError{StatusCode: 403, Message: "The provided load balancer is invalid."}}
	resp := readDS(t, internalprovider.NewLoadBalancerDataSourceWithGetter(getter), map[string]tftypes.Value{"slug": strVal("missing")})
	if !resp.Diagnostics.HasError() || resp.Diagnostics.Errors()[0].Summary() != "Load balancer not found" {
		t.Fatalf("diagnostics = %v, want load balancer not found", resp.Diagnostics)
	}
}

func TestLoadBalancerDataSource_forbiddenIsReadError(t *testing.T) {
	getter := &fakeLoadBalancerGetter{err: &apierrors.APIError{StatusCode: 403, Message: "Access denied."}}
	resp := readDS(t, internalprovider.NewLoadBalancerDataSourceWithGetter(getter), map[string]tftypes.Value{"slug": strVal("web-lb-a1b2")})
	if !resp.Diagnostics.HasError() || resp.Diagnostics.Errors()[0].Summary() != "Failed to read load balancer" {
		t.Fatalf("diagnostics = %v, want failed read", resp.Diagnostics)
	}
}

func TestLoadBalancerDataSource_getError(t *testing.T) {
	getter := &fakeLoadBalancerGetter{err: errors.New("service unavailable")}
	resp := readDS(t, internalprovider.NewLoadBalancerDataSourceWithGetter(getter), map[string]tftypes.Value{"slug": strVal("web-lb-a1b2")})
	if !resp.Diagnostics.HasError() || resp.Diagnostics.Errors()[0].Summary() != "Failed to read load balancer" {
		t.Fatalf("diagnostics = %v, want get error", resp.Diagnostics)
	}
}
