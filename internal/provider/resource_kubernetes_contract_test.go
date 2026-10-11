package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zsoftly/zcp-cli/pkg/api/kubernetes"
	"github.com/zsoftly/zcp-cli/pkg/httpclient"
)

func TestKubernetesCancelUsesOfferingBillingCycle(t *testing.T) {
	var body map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/billing/service-cancel-requests/cluster" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"status":"Success"}`))
	}))
	defer server.Close()
	svc := &kubernetesService{client: httpclient.New(httpclient.Options{BaseURL: server.URL})}
	cluster := &kubernetes.Cluster{Slug: "cluster", Offering: &kubernetes.Offering{BillingCycle: &kubernetes.BillingCycle{Slug: "hourly"}}}
	if err := svc.Cancel(context.Background(), cluster); err != nil {
		t.Fatal(err)
	}
	if body["billing_cycle"] != "hour" || body["type"] != "Immediate" || body["service_name"] != "Kubernetes" {
		t.Fatalf("payload = %#v", body)
	}
}

func TestKubernetesCancelOmitsUnknownBillingCycle(t *testing.T) {
	var body map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"status":"Success"}`))
	}))
	defer server.Close()
	svc := &kubernetesService{client: httpclient.New(httpclient.Options{BaseURL: server.URL})}
	if err := svc.Cancel(context.Background(), &kubernetes.Cluster{Slug: "cluster", BillingCycle: &kubernetes.BillingCycle{Unit: "fortnight"}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["billing_cycle"]; ok {
		t.Fatalf("unexpected billing cycle: %#v", body)
	}
}
