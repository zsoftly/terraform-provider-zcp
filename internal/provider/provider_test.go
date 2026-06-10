package provider_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"zcp": providerserver.NewProtocol6WithError(internalprovider.New("test")()),
}

func TestProvider_instantiates(t *testing.T) {
	_, err := testAccProtoV6ProviderFactories["zcp"]()
	if err != nil {
		t.Fatalf("unexpected error creating provider: %s", err)
	}
}

// configureWith calls Configure on a fresh provider instance with the given
// attribute overrides. Pass nil for an attribute to leave it null.
func configureWith(t *testing.T, bearerToken, apiURL, defaultProject *string) *provider.ConfigureResponse {
	t.Helper()

	p := internalprovider.New("test")()

	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)

	attrs := map[string]tftypes.Value{
		"bearer_token":    tftypes.NewValue(tftypes.String, nil),
		"api_url":         tftypes.NewValue(tftypes.String, nil),
		"default_project": tftypes.NewValue(tftypes.String, nil),
	}
	if bearerToken != nil {
		attrs["bearer_token"] = tftypes.NewValue(tftypes.String, *bearerToken)
	}
	if apiURL != nil {
		attrs["api_url"] = tftypes.NewValue(tftypes.String, *apiURL)
	}
	if defaultProject != nil {
		attrs["default_project"] = tftypes.NewValue(tftypes.String, *defaultProject)
	}

	tfType := schemaResp.Schema.Type().TerraformType(context.Background())
	configVal := tftypes.NewValue(tfType, attrs)

	req := provider.ConfigureRequest{
		Config: tfsdk.Config{
			Schema: schemaResp.Schema,
			Raw:    configVal,
		},
	}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)
	return resp
}

func strPtr(s string) *string { return &s }

// AC: missing bearer_token with no env var → clear error.
func TestProviderConfigure_missingBearerToken(t *testing.T) {
	t.Setenv("ZCP_BEARER_TOKEN", "")

	resp := configureWith(t, nil, nil, nil)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when bearer_token is absent and ZCP_BEARER_TOKEN is unset")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Missing ZCP bearer token" {
		t.Fatalf("unexpected error summary: %q", got)
	}
}

// AC: ZCP_BEARER_TOKEN env var alone is sufficient; no HCL attribute needed.
func TestProviderConfigure_bearerTokenFromEnv(t *testing.T) {
	t.Setenv("ZCP_BEARER_TOKEN", "tok-from-env")

	resp := configureWith(t, nil, nil, nil)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if resp.ResourceData == nil {
		t.Fatal("expected ResourceData to be set")
	}
}

// AC: full HCL config with all attributes wires ProviderData correctly.
func TestProviderConfigure_fullConfig(t *testing.T) {
	t.Setenv("ZCP_BEARER_TOKEN", "")

	resp := configureWith(t,
		strPtr("tok-from-hcl"),
		strPtr("https://staging-api.zcp.zsoftly.ca/api"),
		strPtr("my-project"),
	)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	pd, ok := resp.ResourceData.(*internalprovider.ProviderData)
	if !ok || pd == nil {
		t.Fatal("ResourceData is not *ProviderData")
	}
	if pd.Client == nil {
		t.Fatal("expected Client to be non-nil")
	}
	if pd.DefaultProject != "my-project" {
		t.Fatalf("DefaultProject = %q, want %q", pd.DefaultProject, "my-project")
	}
}

// AC: default_project from env flows through when HCL attribute is omitted.
func TestProviderConfigure_defaultProjectFromEnv(t *testing.T) {
	t.Setenv("ZCP_BEARER_TOKEN", "tok")
	t.Setenv("ZCP_PROJECT", "env-project")

	resp := configureWith(t, nil, nil, nil)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	pd := resp.ResourceData.(*internalprovider.ProviderData)
	if pd.DefaultProject != "env-project" {
		t.Fatalf("DefaultProject = %q, want %q", pd.DefaultProject, "env-project")
	}
}
