package provider_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// This lifecycle uses local HTTP doubles and never provisions a store or bucket.
func TestBucketEncryptionMockLifecycle(t *testing.T) {
	var mu sync.Mutex
	configuration := ""
	puts, deletes := 0, 0
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if strings.TrimSuffix(r.URL.Path, "/") != "/media" || !r.URL.Query().Has("encryption") {
			t.Errorf("unexpected S3 request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			configuration = string(body)
			puts++
		case http.MethodGet:
			if configuration == "" {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `<Error><Code>ServerSideEncryptionConfigurationNotFoundError</Code><Message>No default</Message></Error>`)
				return
			}
			fmt.Fprint(w, configuration)
		case http.MethodDelete:
			configuration = ""
			deletes++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected S3 method: %s", r.Method)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer s3.Close()
	mgmt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected management write: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/object-storages/assets-x1":
			fmt.Fprintf(w, `{"status":"Success","data":{"slug":"assets-x1","region":{"slug":"os-test","cloud_provider_setup":{"config":{"s3_endpoint":%q}}}}}`, s3.URL)
		case "/object-storages/assets-x1/keys":
			fmt.Fprint(w, `{"status":"Success","data":[{"api_key":"test-access","status":"active"}]}`)
		case "/object-storages/assets-x1/buckets/media-b1":
			fmt.Fprint(w, `{"status":"Success","data":{"slug":"media-b1","name":"media"}}`)
		default:
			t.Errorf("unexpected management request: %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mgmt.Close()
	t.Setenv("TF_ACC_PROVIDER_NAMESPACE", "zsoftly")
	t.Setenv("TF_ACC_PROVIDER_HOST", "registry.terraform.io")
	t.Setenv("ZCP_S3_ACCESS_KEY", "test-access")
	t.Setenv("ZCP_S3_SECRET_KEY", "test-secret")
	emptyKey, kmsKey := "", "service-key"
	config := func(algorithm string, key *string) string {
		keyAttribute := ""
		if key != nil {
			keyAttribute = fmt.Sprintf("kms_key_id = %q", *key)
		}
		return fmt.Sprintf(`
terraform {
  required_providers {
    zcp = { source = "registry.terraform.io/zsoftly/zcp" }
  }
}
provider "zcp" {
  api_url = %q
  bearer_token = "test-token"
}
resource "zcp_object_storage_bucket_encryption" "test" {
  object_storage = "assets-x1"
  bucket = "media-b1"
  algorithm = %q
  %s
}
`, mgmt.URL, algorithm, keyAttribute)
	}
	name := "zcp_object_storage_bucket_encryption.test"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(_ *terraform.State) error {
			mu.Lock()
			defer mu.Unlock()
			if configuration != "" || deletes == 0 || puts != 3 {
				return fmt.Errorf("default not removed or writes missing: puts=%d deletes=%d", puts, deletes)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: config("AES256", &emptyKey), PlanOnly: true, ExpectError: regexp.MustCompile("kms_key_id must not be empty")},
			{Config: config("AES256", nil), Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(name, "id", "assets-x1/media-b1"),
				resource.TestCheckResourceAttr(name, "algorithm", "AES256"),
				resource.TestCheckNoResourceAttr(name, "kms_key_id"),
			)},
			{Config: config("AES256", nil), PlanOnly: true},
			{Config: config("AES256", &emptyKey), PlanOnly: true, ExpectError: regexp.MustCompile("kms_key_id must not be empty")},
			{Config: config("AES256", nil), PlanOnly: true},
			{ResourceName: name, ImportState: true, ImportStateVerify: true},
			{Config: config("aws:kms", &kmsKey), Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(name, "algorithm", "aws:kms"),
				resource.TestCheckResourceAttr(name, "kms_key_id", "service-key"),
			)},
			{ResourceName: name, ImportState: true, ImportStateVerify: true},
			{PreConfig: func() { mu.Lock(); configuration = ""; mu.Unlock() }, Config: config("aws:kms", &kmsKey), PlanOnly: true, ExpectNonEmptyPlan: true},
			{Config: config("aws:kms", &kmsKey), Check: func(_ *terraform.State) error {
				mu.Lock()
				defer mu.Unlock()
				if !strings.Contains(configuration, "<SSEAlgorithm>aws:kms</SSEAlgorithm>") || !strings.Contains(configuration, "<KMSMasterKeyID>service-key</KMSMasterKeyID>") {
					return fmt.Errorf("gateway configuration does not match KMS selection")
				}
				return nil
			}},
		},
	})
}
