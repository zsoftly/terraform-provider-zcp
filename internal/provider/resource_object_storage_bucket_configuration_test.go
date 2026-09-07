package provider_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/objectstorage"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

func bucketConfigurationSchema(t *testing.T, kind string) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewObjectStorageBucketConfigurationResourceWithService(&fakeObjectStorageService{}, kind)
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	return resp
}

func bucketConfigurationRaw(t *testing.T, sch resource.SchemaResponse, kind string, enabled bool) tftypes.Value {
	t.Helper()
	typeOf := sch.Schema.Type().TerraformType(context.Background())
	v := map[string]tftypes.Value{}
	for name, attr := range sch.Schema.Attributes {
		typeForAttribute := attr.GetType().TerraformType(context.Background())
		v[name] = tftypes.NewValue(typeForAttribute, nil)
	}
	v["object_storage"] = tftypes.NewValue(tftypes.String, "assets-x1")
	v["bucket"] = tftypes.NewValue(tftypes.String, "media-b1")
	switch kind {
	case "versioning":
		v["enabled"] = tftypes.NewValue(tftypes.Bool, enabled)
	case "policy":
		v["policy"] = tftypes.NewValue(tftypes.String, `{"Version":"2012-10-17","Statement":[]}`)
	case "tagging":
		v["tags"] = tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, map[string]tftypes.Value{"environment": tftypes.NewValue(tftypes.String, "test")})
	case "lifecycle":
		v["prefix"] = tftypes.NewValue(tftypes.String, "uploads/")
		v["days"] = tftypes.NewValue(tftypes.Number, 30)
		v["noncurrent_days"] = tftypes.NewValue(tftypes.Number, 7)
		v["abort_incomplete_multipart_upload_days"] = tftypes.NewValue(tftypes.Number, 1)
	case "cors":
		list := func(values []string) tftypes.Value {
			items := make([]tftypes.Value, len(values))
			for i, value := range values {
				items[i] = tftypes.NewValue(tftypes.String, value)
			}
			return tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, items)
		}
		v["allowed_origins"] = list([]string{"https://example.test"})
		v["allowed_methods"] = list([]string{"GET"})
		v["allowed_headers"] = list([]string{"Authorization"})
		v["max_age_seconds"] = tftypes.NewValue(tftypes.Number, 300)
	}
	return tftypes.NewValue(typeOf, v)
}

func TestBucketConfigurationResourceCRUDAndDrift(t *testing.T) {
	for _, kind := range []string{"versioning", "policy", "tagging", "lifecycle", "cors"} {
		t.Run(kind, func(t *testing.T) {
			svc := &fakeObjectStorageService{bucket: &objectstorage.Bucket{Slug: "media-b1", Name: "media"}, tags: map[string]string{}}
			r := internalprovider.NewObjectStorageBucketConfigurationResourceWithService(svc, kind)
			sch := bucketConfigurationSchema(t, kind)
			typeOf := sch.Schema.Type().TerraformType(context.Background())
			plan := bucketConfigurationRaw(t, sch, kind, true)
			createResp := &resource.CreateResponse{State: tfsdk.State{Schema: sch.Schema, Raw: tftypes.NewValue(typeOf, nil)}}
			r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: sch.Schema, Raw: plan}}, createResp)
			if createResp.Diagnostics.HasError() {
				t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
			}
			assertBucketConfigurationApplied(t, kind, svc)
			if svc.versioning != "Enabled" && kind == "versioning" {
				t.Fatalf("versioning = %q, want Enabled", svc.versioning)
			}

			if kind == "versioning" {
				svc.versioning = "Suspended"
			}
			if kind == "policy" {
				svc.policy = ""
			}
			if kind == "tagging" {
				svc.tags = map[string]string{}
			}
			if kind == "lifecycle" {
				svc.lifecycle = ""
			}
			if kind == "cors" {
				svc.cors = ""
			}
			readResp := &resource.ReadResponse{State: tfsdk.State{Schema: sch.Schema, Raw: createResp.State.Raw}}
			r.Read(context.Background(), resource.ReadRequest{State: tfsdk.State{Schema: sch.Schema, Raw: createResp.State.Raw}}, readResp)
			if readResp.Diagnostics.HasError() {
				t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
			}

			updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch.Schema, Raw: readResp.State.Raw}}
			r.Update(context.Background(), resource.UpdateRequest{Plan: tfsdk.Plan{Schema: sch.Schema, Raw: plan}, State: tfsdk.State{Schema: sch.Schema, Raw: readResp.State.Raw}}, updateResp)
			if updateResp.Diagnostics.HasError() {
				t.Fatalf("update diagnostics: %v", updateResp.Diagnostics)
			}
			assertBucketConfigurationApplied(t, kind, svc)
			deleteResp := &resource.DeleteResponse{}
			r.Delete(context.Background(), resource.DeleteRequest{State: tfsdk.State{Schema: sch.Schema, Raw: updateResp.State.Raw}}, deleteResp)
			if deleteResp.Diagnostics.HasError() {
				t.Fatalf("delete diagnostics: %v", deleteResp.Diagnostics)
			}
			if kind == "lifecycle" && svc.lifecycleSet != nil {
				t.Fatal("lifecycle configuration was not removed")
			}
			if kind == "cors" && svc.corsSet != nil {
				t.Fatal("CORS configuration was not removed")
			}
		})
	}
}

func assertBucketConfigurationApplied(t *testing.T, kind string, svc *fakeObjectStorageService) {
	t.Helper()
	switch kind {
	case "lifecycle":
		want := &fakeLifecycleRequest{prefix: "uploads/", days: 30, noncurrentDays: 7, abortMultipartDays: 1}
		if !reflect.DeepEqual(svc.lifecycleSet, want) {
			t.Fatalf("lifecycle request = %#v, want %#v", svc.lifecycleSet, want)
		}
	case "cors":
		want := &fakeCORSRequest{origins: []string{"https://example.test"}, methods: []string{"GET"}, headers: []string{"Authorization"}, maxAgeSeconds: 300}
		if !reflect.DeepEqual(svc.corsSet, want) {
			t.Fatalf("CORS request = %#v, want %#v", svc.corsSet, want)
		}
	}
}

func TestBucketConfigurationResourceReadNotFoundRemoves(t *testing.T) {
	svc := &fakeObjectStorageService{}
	r := internalprovider.NewObjectStorageBucketConfigurationResourceWithService(svc, "versioning")
	sch := bucketConfigurationSchema(t, "versioning")
	state := bucketConfigurationRaw(t, sch, "versioning", true)
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch.Schema, Raw: state}}
	r.Read(context.Background(), resource.ReadRequest{State: tfsdk.State{Schema: sch.Schema, Raw: state}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected state removal for missing bucket")
	}
}
