package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zsoftly/zcp-cli/pkg/api/instance"
	"github.com/zsoftly/zcp-cli/pkg/httpclient"
)

func TestInstanceServiceChangeHostnameUsesSDKContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.URL.Path != "/virtual-machines/vm-1/change-hostname" {
			http.NotFound(w, req)
			return
		}
		var body instance.ChangeLabelRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if body.Name != "before" || body.Hostname != "after" {
			t.Errorf("request = %#v, want name before and hostname after", body)
		}
		_ = json.NewEncoder(w).Encode(instance.ActionResponse{Status: "Success"})
	}))
	defer server.Close()

	client := httpclient.New(httpclient.Options{BaseURL: server.URL, BearerToken: "test-token"})
	svc := &instanceService{Service: instance.NewService(client), client: client}
	if _, err := svc.ChangeHostname(context.Background(), "vm-1", instance.ChangeLabelRequest{
		Name:     "before",
		Hostname: "after",
	}); err != nil {
		t.Fatalf("ChangeHostname() error = %v", err)
	}
}
