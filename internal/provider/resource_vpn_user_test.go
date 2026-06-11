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
	"github.com/zsoftly/zcp-cli/pkg/api/vpn"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeVPNUserService satisfies vpnUserServiceIface.
type fakeVPNUserService struct {
	users   []vpn.User
	created *vpn.User
	err     error
	deleted []string
}

func (f *fakeVPNUserService) List(_ context.Context) ([]vpn.User, error) {
	return f.users, f.err
}
func (f *fakeVPNUserService) Create(_ context.Context, _ vpn.UserCreateRequest) (*vpn.User, error) {
	return f.created, f.err
}
func (f *fakeVPNUserService) Delete(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	return f.err
}

// vpnUserStateModel mirrors vpnUserResourceModel for state extraction in tests.
type vpnUserStateModel struct {
	ID            types.String   `tfsdk:"id"`
	Username      types.String   `tfsdk:"username"`
	Password      types.String   `tfsdk:"password"`
	CloudProvider types.String   `tfsdk:"cloud_provider"`
	Region        types.String   `tfsdk:"region"`
	Project       types.String   `tfsdk:"project"`
	Status        types.String   `tfsdk:"status"`
	Timeouts      timeouts.Value `tfsdk:"timeouts"`
}

func vpnUserSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewVPNUserResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func vpnUserTFType(t *testing.T) tftypes.Type {
	t.Helper()
	s := vpnUserSchema(t)
	return s.Schema.Type().TerraformType(context.Background())
}

// createVPNUser calls Create on a resource wired with svc.
func createVPNUser(t *testing.T, svc *fakeVPNUserService, username, password, cloudProvider, region string) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewVPNUserResourceWithService(svc)
	schResp := vpnUserSchema(t)
	tfType := vpnUserTFType(t)
	planVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"username":       tftypes.NewValue(tftypes.String, username),
		"password":       tftypes.NewValue(tftypes.String, password),
		"cloud_provider": tftypes.NewValue(tftypes.String, cloudProvider),
		"region":         tftypes.NewValue(tftypes.String, region),
		"project":        tftypes.NewValue(tftypes.String, nil),
		"status":         tftypes.NewValue(tftypes.String, nil),
		"timeouts":       timeoutsNull(t, schResp),
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

// readVPNUser calls Read on a resource wired with svc, with the given slug as current state.
func readVPNUser(t *testing.T, svc *fakeVPNUserService, slug string) resource.ReadResponse {
	t.Helper()
	r := internalprovider.NewVPNUserResourceWithService(svc)
	schResp := vpnUserSchema(t)
	tfType := vpnUserTFType(t)
	stateVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, slug),
		"username":       tftypes.NewValue(tftypes.String, "alice"),
		"password":       tftypes.NewValue(tftypes.String, "s3cr3t"),
		"cloud_provider": tftypes.NewValue(tftypes.String, "nimbo"),
		"region":         tftypes.NewValue(tftypes.String, "yow-1"),
		"project":        tftypes.NewValue(tftypes.String, nil),
		"status":         tftypes.NewValue(tftypes.String, "Enabled"),
		"timeouts":       timeoutsNull(t, schResp),
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

// deleteVPNUser calls Delete on a resource wired with svc.
func deleteVPNUser(t *testing.T, svc *fakeVPNUserService, slug string) resource.DeleteResponse {
	t.Helper()
	r := internalprovider.NewVPNUserResourceWithService(svc)
	schResp := vpnUserSchema(t)
	tfType := vpnUserTFType(t)
	stateVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, slug),
		"username":       tftypes.NewValue(tftypes.String, "alice"),
		"password":       tftypes.NewValue(tftypes.String, "s3cr3t"),
		"cloud_provider": tftypes.NewValue(tftypes.String, "nimbo"),
		"region":         tftypes.NewValue(tftypes.String, "yow-1"),
		"project":        tftypes.NewValue(tftypes.String, nil),
		"status":         tftypes.NewValue(tftypes.String, "Enabled"),
		"timeouts":       timeoutsNull(t, schResp),
	})
	deleteReq := resource.DeleteRequest{
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

func TestVPNUserResource_createHappyPath(t *testing.T) {
	svc := &fakeVPNUserService{
		created: &vpn.User{
			Slug:     "alice",
			UserName: "alice",
			Status:   "Enabled",
		},
	}
	resp := createVPNUser(t, svc, "alice", "s3cr3t", "nimbo", "yow-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got vpnUserStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "alice" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "alice")
	}
	if got.Status.ValueString() != "Enabled" {
		t.Errorf("Status = %q, want %q", got.Status.ValueString(), "Enabled")
	}
}

func TestVPNUserResource_createServiceError(t *testing.T) {
	svc := &fakeVPNUserService{err: errors.New("quota exceeded")}
	resp := createVPNUser(t, svc, "alice", "s3cr3t", "nimbo", "yow-1")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func TestVPNUserResource_readFound(t *testing.T) {
	svc := &fakeVPNUserService{
		users: []vpn.User{
			{Slug: "alice", UserName: "alice", Status: "Enabled"},
		},
	}
	resp := readVPNUser(t, svc, "alice")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got vpnUserStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Username.ValueString() != "alice" {
		t.Errorf("Username = %q, want %q", got.Username.ValueString(), "alice")
	}
	if got.Status.ValueString() != "Enabled" {
		t.Errorf("Status = %q, want %q", got.Status.ValueString(), "Enabled")
	}
	// password is write-only; must be preserved from state.
	if got.Password.ValueString() != "s3cr3t" {
		t.Errorf("Password = %q, want preserved value %q", got.Password.ValueString(), "s3cr3t")
	}
	// cloud_provider is write-only; must be preserved from state.
	if got.CloudProvider.ValueString() != "nimbo" {
		t.Errorf("CloudProvider = %q, want preserved value %q", got.CloudProvider.ValueString(), "nimbo")
	}
}

func TestVPNUserResource_readNotFound(t *testing.T) {
	svc := &fakeVPNUserService{users: []vpn.User{}}
	resp := readVPNUser(t, svc, "missing-slug")
	if resp.Diagnostics.HasError() {
		t.Fatalf("read-not-found should not produce diagnostics: %v", resp.Diagnostics)
	}
	// RemoveResource sets the state to null.
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestVPNUserResource_deleteHappyPath(t *testing.T) {
	svc := &fakeVPNUserService{}
	resp := deleteVPNUser(t, svc, "alice")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "alice" {
		t.Errorf("Delete called with %v, want [alice]", svc.deleted)
	}
}

func TestVPNUserResource_delete404IsNoOp(t *testing.T) {
	svc := &fakeVPNUserService{err: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := deleteVPNUser(t, svc, "alice")
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 on delete should be a no-op: %v", resp.Diagnostics)
	}
}
