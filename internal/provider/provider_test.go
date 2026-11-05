package provider_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	server, err := testAccProtoV6ProviderFactories["zcp"]()
	if err != nil {
		t.Fatalf("unexpected error creating provider: %s", err)
	}
	resp, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("unexpected error loading provider schema: %s", err)
	}
	if len(resp.Diagnostics) != 0 {
		t.Fatalf("unexpected provider schema diagnostics: %v", resp.Diagnostics)
	}
}

// configureWith calls Configure on a fresh provider instance with the given
// attribute overrides. Pass nil for an attribute to leave it null.
func configureWith(t *testing.T, bearerToken, apiURL, defaultProject, profile *string) *provider.ConfigureResponse {
	t.Helper()

	p := internalprovider.New("test")()

	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)

	attrs := map[string]tftypes.Value{
		"bearer_token":    tftypes.NewValue(tftypes.String, nil),
		"api_url":         tftypes.NewValue(tftypes.String, nil),
		"default_project": tftypes.NewValue(tftypes.String, nil),
		"profile":         tftypes.NewValue(tftypes.String, nil),
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
	if profile != nil {
		attrs["profile"] = tftypes.NewValue(tftypes.String, *profile)
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

func useEmptyCLIConfig(t *testing.T) {
	t.Helper()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("APPDATA", configHome)
	t.Setenv("ZCP_PROFILE", "")
}

func writeCLIConfig(t *testing.T, content string) {
	t.Helper()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("APPDATA", configHome)
	t.Setenv("ZCP_PROFILE", "")
	configDir := filepath.Join(configHome, "zcp")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("creating test CLI config directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(content), 0o600); err != nil {
		t.Fatalf("writing test CLI config: %v", err)
	}
}

// AC: missing bearer_token with no env var → clear error.
func TestProviderConfigure_missingBearerToken(t *testing.T) {
	useEmptyCLIConfig(t)
	t.Setenv("ZCP_BEARER_TOKEN", "")

	resp := configureWith(t, nil, nil, nil, nil)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when bearer_token is absent and ZCP_BEARER_TOKEN is unset")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Missing ZCP bearer token" {
		t.Fatalf("unexpected error summary: %q", got)
	}
}

// AC: ZCP_BEARER_TOKEN env var alone is sufficient; no HCL attribute needed.
func TestProviderConfigure_bearerTokenFromEnv(t *testing.T) {
	useEmptyCLIConfig(t)
	t.Setenv("ZCP_BEARER_TOKEN", "tok-from-env")

	resp := configureWith(t, nil, nil, nil, nil)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if resp.ResourceData == nil {
		t.Fatal("expected ResourceData to be set")
	}
}

// AC: full HCL config with all attributes wires ProviderData correctly.
func TestProviderConfigure_fullConfig(t *testing.T) {
	useEmptyCLIConfig(t)
	t.Setenv("ZCP_BEARER_TOKEN", "")

	resp := configureWith(t,
		strPtr("tok-from-hcl"),
		strPtr("https://staging-api.zcp.zsoftly.ca/api"),
		strPtr("my-project"),
		nil,
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
	useEmptyCLIConfig(t)
	t.Setenv("ZCP_BEARER_TOKEN", "tok")
	t.Setenv("ZCP_PROJECT", "env-project")

	resp := configureWith(t, nil, nil, nil, nil)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	pd := resp.ResourceData.(*internalprovider.ProviderData)
	if pd.DefaultProject != "env-project" {
		t.Fatalf("DefaultProject = %q, want %q", pd.DefaultProject, "env-project")
	}
}

func TestProviderConfigure_activeCLIProfile(t *testing.T) {
	writeCLIConfig(t, `active_profile: development
profiles:
  development:
    bearer_token: token-from-profile
    api_url: https://development.zcp.example/api
    project: development-project
`)
	t.Setenv("ZCP_BEARER_TOKEN", "token-from-environment")

	resp := configureWith(t, nil, nil, nil, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	pd := resp.ResourceData.(*internalprovider.ProviderData)
	if pd.DefaultProject != "development-project" {
		t.Fatalf("DefaultProject = %q, want profile project", pd.DefaultProject)
	}
}

func TestProviderConfigure_credentialPrecedenceUsesSelectedToken(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	writeCLIConfig(t, fmt.Sprintf(`active_profile: development
profiles:
  development:
    bearer_token: token-from-profile
    api_url: %s
`, server.URL))

	tests := []struct {
		name        string
		providerTok *string
		envToken    string
		want        string
	}{
		{name: "provider configuration", providerTok: strPtr("token-from-provider"), envToken: "token-from-environment", want: "Bearer token-from-provider"},
		{name: "environment", envToken: "token-from-environment", want: "Bearer token-from-environment"},
		{name: "active profile", want: "Bearer token-from-profile"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ZCP_BEARER_TOKEN", tt.envToken)
			authorization = ""
			resp := configureWith(t, tt.providerTok, nil, nil, nil)
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected error: %v", resp.Diagnostics)
			}
			pd := resp.ResourceData.(*internalprovider.ProviderData)
			if err := pd.Client.Get(context.Background(), "/", nil, nil); err != nil {
				t.Fatalf("calling profile API URL: %v", err)
			}
			if authorization != tt.want {
				t.Fatalf("Authorization = %q, want %q", authorization, tt.want)
			}
		})
	}
}

func TestProviderConfigure_staleActiveProfileDoesNotBlockEnvironmentToken(t *testing.T) {
	writeCLIConfig(t, `active_profile: removed
profiles: {}
`)
	t.Setenv("ZCP_BEARER_TOKEN", "token-from-environment")

	resp := configureWith(t, nil, nil, nil, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
}

func TestProviderConfigure_staleActiveProfileBlocksEmptyProviderToken(t *testing.T) {
	writeCLIConfig(t, `active_profile: removed
profiles: {}
`)
	t.Setenv("ZCP_BEARER_TOKEN", "")

	resp := configureWith(t, strPtr(""), nil, nil, nil)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected stale active profile to block an empty provider token")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Invalid active ZCP CLI profile" {
		t.Fatalf("unexpected error summary: %q", got)
	}
}

func TestProviderConfigure_environmentAPIURLOverridesActiveProfile(t *testing.T) {
	profileCalled := false
	profileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		profileCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer profileServer.Close()
	environmentCalled := false
	environmentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		environmentCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer environmentServer.Close()

	writeCLIConfig(t, fmt.Sprintf(`active_profile: development
profiles:
  development:
    bearer_token: profile-token
    api_url: %s
    project: profile-project
`, profileServer.URL))
	t.Setenv("ZCP_BEARER_TOKEN", "environment-token")
	t.Setenv("ZCP_API_URL", environmentServer.URL)

	resp := configureWith(t, nil, nil, nil, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	pd := resp.ResourceData.(*internalprovider.ProviderData)
	if err := pd.Client.Get(context.Background(), "/", nil, nil); err != nil {
		t.Fatalf("calling environment API URL: %v", err)
	}
	if !environmentCalled || profileCalled {
		t.Fatalf("environmentCalled = %t, profileCalled = %t, want true and false", environmentCalled, profileCalled)
	}
}

func TestProviderConfigure_environmentProjectOverridesActiveProfile(t *testing.T) {
	writeCLIConfig(t, `active_profile: development
profiles:
  development:
    bearer_token: profile-token
    project: profile-project
`)
	t.Setenv("ZCP_BEARER_TOKEN", "environment-token")
	t.Setenv("ZCP_PROJECT", "environment-project")

	resp := configureWith(t, nil, nil, nil, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	pd := resp.ResourceData.(*internalprovider.ProviderData)
	if pd.DefaultProject != "environment-project" {
		t.Fatalf("DefaultProject = %q, want environment project", pd.DefaultProject)
	}
}

func TestProviderConfigure_malformedCLIConfigBlocksEnvironmentToken(t *testing.T) {
	const secret = "SECRET123"
	writeCLIConfig(t, "active_profile: bearer_token\nprofiles:\n  bearer_token: "+secret+"\n")
	t.Setenv("ZCP_BEARER_TOKEN", "token-from-environment")

	resp := configureWith(t, nil, nil, nil, nil)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected malformed CLI configuration to fail")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Invalid active ZCP CLI profile" {
		t.Fatalf("unexpected error summary: %q", got)
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); strings.Contains(detail, secret) {
		t.Fatalf("diagnostic detail leaked a token: %q", detail)
	}
}

func TestProviderConfigure_malformedSelectedCLIProfileDoesNotLeakToken(t *testing.T) {
	const secret = "SECRET123"
	writeCLIConfig(t, "profiles:\n  bearer_token: "+secret+"\n")
	t.Setenv("ZCP_PROFILE", "staging")
	t.Setenv("ZCP_BEARER_TOKEN", "token-from-environment")

	resp := configureWith(t, nil, nil, nil, nil)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected malformed selected CLI profile to fail")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Invalid ZCP CLI profile" {
		t.Fatalf("unexpected error summary: %q", got)
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); strings.Contains(detail, secret) {
		t.Fatalf("diagnostic detail leaked a token: %q", detail)
	}
}

func TestProviderConfigure_unknownProfileFailsBeforeCredentialResolution(t *testing.T) {
	useEmptyCLIConfig(t)
	t.Setenv("ZCP_BEARER_TOKEN", "token-from-environment")

	p := internalprovider.New("test")()
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	attrs := map[string]tftypes.Value{
		"bearer_token":    tftypes.NewValue(tftypes.String, nil),
		"api_url":         tftypes.NewValue(tftypes.String, nil),
		"default_project": tftypes.NewValue(tftypes.String, nil),
		"profile":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	}
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), attrs)
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), provider.ConfigureRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: configVal}}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected unknown profile to fail before environment credential resolution")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Unknown ZCP CLI profile" {
		t.Fatalf("unexpected error summary: %q", got)
	}
}

func TestProviderConfigure_unknownBearerTokenFailsBeforeCredentialResolution(t *testing.T) {
	useEmptyCLIConfig(t)
	t.Setenv("ZCP_BEARER_TOKEN", "token-from-environment")

	p := internalprovider.New("test")()
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	attrs := map[string]tftypes.Value{
		"bearer_token":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"api_url":         tftypes.NewValue(tftypes.String, nil),
		"default_project": tftypes.NewValue(tftypes.String, nil),
		"profile":         tftypes.NewValue(tftypes.String, nil),
	}
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), attrs)
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), provider.ConfigureRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: configVal}}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected unknown bearer_token to fail before environment credential resolution")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Unknown ZCP bearer token" {
		t.Fatalf("unexpected error summary: %q", got)
	}
}

func TestProviderConfigure_unknownAPIURLFailsBeforeEnvironmentFallback(t *testing.T) {
	useEmptyCLIConfig(t)
	t.Setenv("ZCP_BEARER_TOKEN", "token-from-environment")
	t.Setenv("ZCP_API_URL", "https://environment.zcp.example/api")

	p := internalprovider.New("test")()
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	attrs := map[string]tftypes.Value{
		"bearer_token":    tftypes.NewValue(tftypes.String, nil),
		"api_url":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"default_project": tftypes.NewValue(tftypes.String, nil),
		"profile":         tftypes.NewValue(tftypes.String, nil),
	}
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), attrs)
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), provider.ConfigureRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: configVal}}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected unknown api_url to fail before environment fallback")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Unknown ZCP API URL" {
		t.Fatalf("unexpected error summary: %q", got)
	}
}

func TestProviderConfigure_unknownDefaultProjectFailsBeforeEnvironmentFallback(t *testing.T) {
	useEmptyCLIConfig(t)
	t.Setenv("ZCP_BEARER_TOKEN", "token-from-environment")
	t.Setenv("ZCP_PROJECT", "environment-project")

	p := internalprovider.New("test")()
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	attrs := map[string]tftypes.Value{
		"bearer_token":    tftypes.NewValue(tftypes.String, nil),
		"api_url":         tftypes.NewValue(tftypes.String, nil),
		"default_project": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"profile":         tftypes.NewValue(tftypes.String, nil),
	}
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), attrs)
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), provider.ConfigureRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: configVal}}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected unknown default_project to fail before environment fallback")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Unknown ZCP default project" {
		t.Fatalf("unexpected error summary: %q", got)
	}
}

func TestProviderConfigure_environmentProfileOverridesActiveProfile(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	writeCLIConfig(t, fmt.Sprintf(`active_profile: development
profiles:
  development:
    bearer_token: active-token
    api_url: %s
  staging:
    bearer_token: selected-token
    api_url: %s
`, server.URL, server.URL))
	t.Setenv("ZCP_PROFILE", "staging")
	t.Setenv("ZCP_BEARER_TOKEN", "")

	resp := configureWith(t, nil, nil, nil, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	pd := resp.ResourceData.(*internalprovider.ProviderData)
	if err := pd.Client.Get(context.Background(), "/", nil, nil); err != nil {
		t.Fatalf("calling selected profile API URL: %v", err)
	}
	if authorization != "Bearer selected-token" {
		t.Fatalf("Authorization = %q, want selected profile token", authorization)
	}
}

func TestProviderConfigure_missingEnvironmentProfileFailsBeforeEnvironmentToken(t *testing.T) {
	useEmptyCLIConfig(t)
	t.Setenv("ZCP_PROFILE", "missing")
	t.Setenv("ZCP_BEARER_TOKEN", "token-from-environment")

	resp := configureWith(t, nil, nil, nil, nil)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected missing ZCP_PROFILE to fail even when ZCP_BEARER_TOKEN is set")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Invalid ZCP CLI profile" {
		t.Fatalf("unexpected error summary: %q", got)
	}
}

func TestProviderConfigure_explicitProfileWithoutTokenUsesEnvironmentToken(t *testing.T) {
	writeCLIConfig(t, `profiles:
  staging:
    api_url: https://staging.zcp.example/api
    project: staging-project
`)
	t.Setenv("ZCP_BEARER_TOKEN", "token-from-environment")

	resp := configureWith(t, nil, nil, nil, strPtr("staging"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	pd := resp.ResourceData.(*internalprovider.ProviderData)
	if pd.DefaultProject != "staging-project" {
		t.Fatalf("DefaultProject = %q, want profile project", pd.DefaultProject)
	}
}

func TestProviderConfigure_explicitProfileMissingFailsBeforeEnvironmentToken(t *testing.T) {
	useEmptyCLIConfig(t)
	t.Setenv("ZCP_BEARER_TOKEN", "token-from-env")

	resp := configureWith(t, nil, nil, nil, strPtr("missing"))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected missing explicit profile to fail even when ZCP_BEARER_TOKEN is set")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Invalid ZCP CLI profile" {
		t.Fatalf("unexpected error summary: %q", got)
	}
}

func TestProviderConfigure_providerTokenOverridesEnvironmentAndProfile(t *testing.T) {
	writeCLIConfig(t, `active_profile: development
profiles:
  development:
    bearer_token: token-from-profile
`)
	t.Setenv("ZCP_BEARER_TOKEN", "token-from-env")

	resp := configureWith(t, strPtr("token-from-provider"), nil, nil, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
}
