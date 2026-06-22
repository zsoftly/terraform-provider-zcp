package provider_test

import (
	"os"
	"testing"
	"time"

	"github.com/zsoftly/zcp-cli/pkg/httpclient"
)

// Acceptance tests run only when TF_ACC is set (the terraform-plugin-testing
// helper enforces this and skips otherwise). They additionally require a live
// ZCP_BEARER_TOKEN and a handful of account-specific slugs supplied via env
// vars — any test whose inputs are missing skips itself rather than failing, so
// partial environments still exercise what they can.
//
// Required for every acceptance test:
//
//	TF_ACC=1
//	ZCP_BEARER_TOKEN=<token>
//
// Optional overrides (sensible defaults shown):
//
//	ZCP_API_URL                 (default https://api.zcp.zsoftly.ca/api)
//	ZCP_ACC_REGION              (default yow-1)
//	ZCP_ACC_PROJECT             (provider default_project if unset)
//	ZCP_ACC_SSH_KEY             (SSH key name to attach, optional)
//
// Per-resource slugs (a test skips when its required slug is absent):
//
//	ZCP_ACC_TEMPLATE            zcp_instance template slug
//	ZCP_ACC_INSTANCE_PLAN       zcp_instance compute plan slug
//	ZCP_ACC_STORAGE_CATEGORY    storage category slug (volume + k8s)
//	ZCP_ACC_VOLUME_PLAN         zcp_volume plan slug
//	ZCP_ACC_K8S_VERSION         zcp_kubernetes_cluster version (e.g. v1.36.1)
//	ZCP_ACC_K8S_PLAN            zcp_kubernetes_cluster plan slug

// testAccPreCheck verifies the baseline credentials common to all acceptance
// tests. Call it from a test's PreCheck closure.
func testAccPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("ZCP_BEARER_TOKEN") == "" {
		t.Fatal("ZCP_BEARER_TOKEN must be set for acceptance tests")
	}
}

// accEnv returns the env var value or, if empty, skips the test with a helpful
// message naming the missing variable.
func accEnv(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("%s not set — skipping (supply it to run this acceptance test)", key)
	}
	return v
}

// accEnvDefault returns the env var value or the supplied default.
func accEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// accRegion returns the region slug under test (default yow-1).
func accRegion() string { return accEnvDefault("ZCP_ACC_REGION", "yow-1") }

// accClient builds a live httpclient from the acceptance env for CheckDestroy
// assertions that query the API directly.
func accClient(t *testing.T) *httpclient.Client {
	t.Helper()
	return httpclient.New(httpclient.Options{
		BaseURL:     accEnvDefault("ZCP_API_URL", "https://api.zcp.zsoftly.ca/api"),
		BearerToken: os.Getenv("ZCP_BEARER_TOKEN"),
		Timeout:     5 * time.Minute,
	})
}
