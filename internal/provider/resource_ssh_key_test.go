package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/sshkey"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeSSHKeyService satisfies sshKeyServiceIface.
type fakeSSHKeyService struct {
	keys    []sshkey.SSHKey
	created *sshkey.SSHKey
	err     error
	deleted []string
}

func (f *fakeSSHKeyService) List(_ context.Context) ([]sshkey.SSHKey, error) {
	return f.keys, f.err
}
func (f *fakeSSHKeyService) Create(_ context.Context, _ sshkey.CreateRequest) (*sshkey.SSHKey, error) {
	return f.created, f.err
}
func (f *fakeSSHKeyService) Delete(_ context.Context, keyID string) error {
	f.deleted = append(f.deleted, keyID)
	return f.err
}

// sshKeyStateModel mirrors sshKeyResourceModel for state extraction in tests.
type sshKeyStateModel struct {
	ID        types.String   `tfsdk:"id"`
	Name      types.String   `tfsdk:"name"`
	PublicKey types.String   `tfsdk:"public_key"`
	Project   types.String   `tfsdk:"project"`
	CreatedAt types.String   `tfsdk:"created_at"`
	Timeouts  timeouts.Value `tfsdk:"timeouts"`
}

func sshKeySchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewSSHKeyResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func sshKeyTFType(t *testing.T) tftypes.Type {
	t.Helper()
	s := sshKeySchema(t)
	return s.Schema.Type().TerraformType(context.Background())
}

// createSSHKey calls Create on a resource wired with svc.
func createSSHKey(t *testing.T, svc *fakeSSHKeyService, name, pubKey string) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewSSHKeyResourceWithService(svc)
	schResp := sshKeySchema(t)
	tfType := sshKeyTFType(t)
	planVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, nil),
		"name":       tftypes.NewValue(tftypes.String, name),
		"public_key": tftypes.NewValue(tftypes.String, pubKey),
		"project":    tftypes.NewValue(tftypes.String, nil),
		"created_at": tftypes.NewValue(tftypes.String, nil),
		"timeouts":   timeoutsNull(t, schResp),
	})
	createReq := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: planVal},
	}
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)},
	}
	r.Create(context.Background(), createReq, createResp)
	return *createResp
}

// readSSHKey calls Read on a resource wired with svc, with the given slug as current state.
func readSSHKey(t *testing.T, svc *fakeSSHKeyService, slug string) resource.ReadResponse {
	t.Helper()
	var r resource.Resource
	if svc != nil {
		r = internalprovider.NewSSHKeyResourceWithService(svc)
	} else {
		r = internalprovider.NewSSHKeyResource()
	}
	schResp := sshKeySchema(t)
	tfType := sshKeyTFType(t)
	stateVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, slug),
		"name":       tftypes.NewValue(tftypes.String, "testkey"),
		"public_key": tftypes.NewValue(tftypes.String, "ssh-rsa AAAA"),
		"project":    tftypes.NewValue(tftypes.String, nil),
		"created_at": tftypes.NewValue(tftypes.String, "2024-01-01T00:00:00Z"),
		"timeouts":   timeoutsNull(t, schResp),
	})
	readReq := resource.ReadRequest{
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	readResp := &resource.ReadResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	r.Read(context.Background(), readReq, readResp)
	return *readResp
}

// deleteSSHKey calls Delete on a resource wired with svc.
func deleteSSHKey(t *testing.T, svc *fakeSSHKeyService, slug string) resource.DeleteResponse {
	t.Helper()
	r := internalprovider.NewSSHKeyResourceWithService(svc)
	schResp := sshKeySchema(t)
	tfType := sshKeyTFType(t)
	stateVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, slug),
		"name":       tftypes.NewValue(tftypes.String, "testkey"),
		"public_key": tftypes.NewValue(tftypes.String, "ssh-rsa AAAA"),
		"project":    tftypes.NewValue(tftypes.String, nil),
		"created_at": tftypes.NewValue(tftypes.String, "2024-01-01T00:00:00Z"),
		"timeouts":   timeoutsNull(t, schResp),
	})
	deleteReq := resource.DeleteRequest{
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

func TestSSHKeyResource_createHappyPath(t *testing.T) {
	svc := &fakeSSHKeyService{
		created: &sshkey.SSHKey{
			Slug:      "mykey-abc123",
			Name:      "mykey",
			CreatedAt: "2024-06-01T10:00:00Z",
		},
	}
	resp := createSSHKey(t, svc, "mykey", "ssh-rsa AAABBB")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got sshKeyStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "mykey-abc123" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "mykey-abc123")
	}
	if got.CreatedAt.ValueString() != "2024-06-01T10:00:00Z" {
		t.Errorf("CreatedAt = %q, want %q", got.CreatedAt.ValueString(), "2024-06-01T10:00:00Z")
	}
}

func TestSSHKeyResource_createServiceError(t *testing.T) {
	svc := &fakeSSHKeyService{err: errors.New("quota exceeded")}
	resp := createSSHKey(t, svc, "mykey", "ssh-rsa AAABBB")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func TestSSHKeyResource_readFound(t *testing.T) {
	svc := &fakeSSHKeyService{
		keys: []sshkey.SSHKey{
			{Slug: "mykey-abc123", Name: "mykey", PublicKey: "ssh-rsa AAAA", CreatedAt: "2024-06-01T10:00:00Z"},
		},
	}
	resp := readSSHKey(t, svc, "mykey-abc123")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got sshKeyStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Name.ValueString() != "mykey" {
		t.Errorf("Name = %q, want %q", got.Name.ValueString(), "mykey")
	}
}

func TestSSHKeyResource_readNotFound(t *testing.T) {
	svc := &fakeSSHKeyService{keys: []sshkey.SSHKey{}}
	resp := readSSHKey(t, svc, "missing-slug")
	if resp.Diagnostics.HasError() {
		t.Fatalf("read-not-found should not produce diagnostics: %v", resp.Diagnostics)
	}
	// RemoveResource sets the state to null.
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestSSHKeyResource_readListError(t *testing.T) {
	svc := &fakeSSHKeyService{err: errors.New("network error")}
	resp := readSSHKey(t, svc, "any-slug")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on list failure, got none")
	}
}

func TestSSHKeyResource_readWithoutConfigure(t *testing.T) {
	resp := readSSHKey(t, nil, "any-slug")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when svc is nil, got none")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Provider not configured" {
		t.Errorf("unexpected summary: %q", got)
	}
}

func TestSSHKeyResource_deleteHappyPath(t *testing.T) {
	svc := &fakeSSHKeyService{}
	resp := deleteSSHKey(t, svc, "mykey-abc123")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "mykey-abc123" {
		t.Errorf("Delete called with %v, want [mykey-abc123]", svc.deleted)
	}
}

func TestSSHKeyResource_delete404IsNoOp(t *testing.T) {
	svc := &fakeSSHKeyService{err: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := deleteSSHKey(t, svc, "gone-slug")
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 on delete should be a no-op: %v", resp.Diagnostics)
	}
}
