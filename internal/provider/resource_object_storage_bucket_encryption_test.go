package provider_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/objectstorage"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

func encryptionRaw(t *testing.T, algorithm, key any) tftypes.Value {
	t.Helper()
	sch := bucketConfigurationSchema(t, "encryption")
	values := map[string]tftypes.Value{}
	for name, attribute := range sch.Schema.Attributes {
		values[name] = tftypes.NewValue(attribute.GetType().TerraformType(context.Background()), nil)
	}
	values["object_storage"] = tftypes.NewValue(tftypes.String, "assets-x1")
	values["bucket"] = tftypes.NewValue(tftypes.String, "media-b1")
	values["algorithm"] = tftypes.NewValue(tftypes.String, algorithm)
	values["kms_key_id"] = tftypes.NewValue(tftypes.String, key)
	return tftypes.NewValue(sch.Schema.Type().TerraformType(context.Background()), values)
}

func TestBucketEncryptionValidation(t *testing.T) {
	for _, tc := range []struct {
		algorithm, key any
		invalid        bool
	}{
		{"AES256", nil, false}, {"aws:kms", "service-key", false},
		{"AES256", "key", true}, {"aws:kms", nil, true},
		{"aws:kms", " ", true}, {"aws:kms", "mount/key", true},
		{"invalid", nil, true}, {"", nil, true},
		{tftypes.UnknownValue, nil, false}, {"aws:kms", tftypes.UnknownValue, false},
	} {
		t.Run(fmt.Sprintf("%v/%v", tc.algorithm, tc.key), func(t *testing.T) {
			r := internalprovider.NewObjectStorageBucketEncryptionResource().(resource.ResourceWithValidateConfig)
			sch := bucketConfigurationSchema(t, "encryption")
			var resp resource.ValidateConfigResponse
			r.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: sch.Schema, Raw: encryptionRaw(t, tc.algorithm, tc.key)}}, &resp)
			if resp.Diagnostics.HasError() != tc.invalid {
				t.Fatalf("diagnostics=%v, want error=%t", resp.Diagnostics, tc.invalid)
			}
		})
	}
}

func TestBucketEncryptionImportAndDrift(t *testing.T) {
	ctx := context.Background()
	sch := bucketConfigurationSchema(t, "encryption")
	svc := &fakeObjectStorageService{bucket: &objectstorage.Bucket{Slug: "media-b1", Name: "media"}, encryption: objectstorage.EncryptionOptions{Algorithm: "aws:kms", KMSKeyID: "service-key"}}
	r := internalprovider.NewObjectStorageBucketConfigurationResourceWithService(svc, "encryption")
	for _, id := range []string{"assets-x1/media-b1", "invalid", "store/", "/bucket", "store/bucket/extra"} {
		resp := resource.ImportStateResponse{State: tfsdk.State{Schema: sch.Schema, Raw: encryptionRaw(t, nil, nil)}}
		r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: id}, &resp)
		if id != "assets-x1/media-b1" {
			if !resp.Diagnostics.HasError() {
				t.Fatalf("import %q accepted", id)
			}
			continue
		}
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
		read := resource.ReadResponse{State: resp.State}
		r.Read(ctx, resource.ReadRequest{State: resp.State}, &read)
		if read.Diagnostics.HasError() {
			t.Fatal(read.Diagnostics)
		}
		var key string
		if d := read.State.GetAttribute(ctx, path.Root("kms_key_id"), &key); d.HasError() || key != "service-key" {
			t.Fatalf("key=%q diagnostics=%v", key, d)
		}
		if svc.encryptionBucket != "media" {
			t.Fatalf("S3 used slug instead of name: %q", svc.encryptionBucket)
		}
		svc.encryption = objectstorage.EncryptionOptions{}
		removed := resource.ReadResponse{State: read.State}
		r.Read(ctx, resource.ReadRequest{State: read.State}, &removed)
		if removed.Diagnostics.HasError() || !removed.State.Raw.IsNull() {
			t.Fatalf("missing default retained: %v", removed.Diagnostics)
		}
	}
}

func TestBucketEncryptionFailedWritesPreserveState(t *testing.T) {
	ctx := context.Background()
	sch := bucketConfigurationSchema(t, "encryption")
	svc := &fakeObjectStorageService{bucket: &objectstorage.Bucket{Slug: "media-b1", Name: "media"}, encryptionErr: fmt.Errorf("regional encryption backend unavailable")}
	r := internalprovider.NewObjectStorageBucketConfigurationResourceWithService(svc, "encryption")
	state := tfsdk.State{Schema: sch.Schema, Raw: encryptionRaw(t, "AES256", nil)}
	update := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{State: state, Plan: tfsdk.Plan{Schema: sch.Schema, Raw: encryptionRaw(t, "aws:kms", "service-key")}}, &update)
	if !update.Diagnostics.HasError() || !update.State.Raw.Equal(state.Raw) {
		t.Fatal("failed update changed state")
	}
	var deleted resource.DeleteResponse
	r.Delete(ctx, resource.DeleteRequest{State: state}, &deleted)
	if !deleted.Diagnostics.HasError() {
		t.Fatal("failed delete reported success")
	}
	var created resource.CreateResponse
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: sch.Schema, Raw: state.Raw}}, &created)
	if !created.Diagnostics.HasError() {
		t.Fatal("failed create reported success")
	}
}
