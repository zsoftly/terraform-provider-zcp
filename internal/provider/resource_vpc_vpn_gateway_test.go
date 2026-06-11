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
	"github.com/zsoftly/zcp-cli/pkg/api/vpc"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeVPNGatewayService satisfies vpcVPNGatewayServiceIface.
type fakeVPNGatewayService struct {
	gateways []vpc.VPNGateway
	created  *vpc.VPNGateway
	err      error
	deleted  []string // "vpcSlug/gatewayID" pairs recorded on Delete
}

func (f *fakeVPNGatewayService) ListVPNGateways(_ context.Context, _ string) ([]vpc.VPNGateway, error) {
	return f.gateways, f.err
}
func (f *fakeVPNGatewayService) CreateVPNGateway(_ context.Context, _ string) (*vpc.VPNGateway, error) {
	return f.created, f.err
}
func (f *fakeVPNGatewayService) DeleteVPNGateway(_ context.Context, vpcSlug, gatewayID string) error {
	f.deleted = append(f.deleted, vpcSlug+"/"+gatewayID)
	return f.err
}

// vpnGatewayStateModel mirrors vpcVPNGatewayResourceModel for state extraction in tests.
type vpnGatewayStateModel struct {
	ID       types.String   `tfsdk:"id"`
	VPC      types.String   `tfsdk:"vpc"`
	PublicIP types.String   `tfsdk:"public_ip"`
	Status   types.String   `tfsdk:"status"`
	ZoneName types.String   `tfsdk:"zone_name"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func vpnGatewaySchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewVPCVPNGatewayResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func vpnGatewayTFType(t *testing.T) tftypes.Type {
	t.Helper()
	return vpnGatewaySchema(t).Schema.Type().TerraformType(context.Background())
}

// createVPNGateway calls Create on a resource wired with svc.
func createVPNGateway(t *testing.T, svc *fakeVPNGatewayService, vpcSlug string) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewVPCVPNGatewayResourceWithService(svc)
	schResp := vpnGatewaySchema(t)
	tfType := vpnGatewayTFType(t)
	planVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, nil),
		"vpc":       tftypes.NewValue(tftypes.String, vpcSlug),
		"public_ip": tftypes.NewValue(tftypes.String, nil),
		"status":    tftypes.NewValue(tftypes.String, nil),
		"zone_name": tftypes.NewValue(tftypes.String, nil),
		"timeouts":  timeoutsNull(t, schResp),
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

// readVPNGateway calls Read on a resource wired with svc, using vpcSlug and gwSlug as current state.
func readVPNGateway(t *testing.T, svc *fakeVPNGatewayService, vpcSlug, gwSlug string) resource.ReadResponse {
	t.Helper()
	r := internalprovider.NewVPCVPNGatewayResourceWithService(svc)
	schResp := vpnGatewaySchema(t)
	tfType := vpnGatewayTFType(t)
	stateVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, gwSlug),
		"vpc":       tftypes.NewValue(tftypes.String, vpcSlug),
		"public_ip": tftypes.NewValue(tftypes.String, "203.0.113.5"),
		"status":    tftypes.NewValue(tftypes.String, "Enabled"),
		"zone_name": tftypes.NewValue(tftypes.String, "YOW-1"),
		"timeouts":  timeoutsNull(t, schResp),
	})
	readReq := resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), readReq, readResp)
	return *readResp
}

// deleteVPNGateway calls Delete on a resource wired with svc.
func deleteVPNGateway(t *testing.T, svc *fakeVPNGatewayService, vpcSlug, gwSlug string) resource.DeleteResponse {
	t.Helper()
	r := internalprovider.NewVPCVPNGatewayResourceWithService(svc)
	schResp := vpnGatewaySchema(t)
	tfType := vpnGatewayTFType(t)
	stateVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, gwSlug),
		"vpc":       tftypes.NewValue(tftypes.String, vpcSlug),
		"public_ip": tftypes.NewValue(tftypes.String, "203.0.113.5"),
		"status":    tftypes.NewValue(tftypes.String, "Enabled"),
		"zone_name": tftypes.NewValue(tftypes.String, "YOW-1"),
		"timeouts":  timeoutsNull(t, schResp),
	})
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

func TestVPCVPNGatewayResource_createHappyPath(t *testing.T) {
	svc := &fakeVPNGatewayService{
		created: &vpc.VPNGateway{
			ID:       "84671917-0c3b-47bd-9996-22c2536ea399",
			PublicIP: "203.0.113.5",
		},
	}
	resp := createVPNGateway(t, svc, "my-vpc")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got vpnGatewayStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "84671917-0c3b-47bd-9996-22c2536ea399" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "84671917-0c3b-47bd-9996-22c2536ea399")
	}
	if got.PublicIP.ValueString() != "203.0.113.5" {
		t.Errorf("PublicIP = %q, want %q", got.PublicIP.ValueString(), "203.0.113.5")
	}
	if got.VPC.ValueString() != "my-vpc" {
		t.Errorf("VPC = %q, want %q", got.VPC.ValueString(), "my-vpc")
	}
}

func TestVPCVPNGatewayResource_createServiceError(t *testing.T) {
	svc := &fakeVPNGatewayService{err: errors.New("quota exceeded")}
	resp := createVPNGateway(t, svc, "my-vpc")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func TestVPCVPNGatewayResource_readFound(t *testing.T) {
	svc := &fakeVPNGatewayService{
		gateways: []vpc.VPNGateway{
			{ID: "gw-uuid-1", PublicIP: "203.0.113.5"},
		},
	}
	resp := readVPNGateway(t, svc, "my-vpc", "gw-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got vpnGatewayStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.PublicIP.ValueString() != "203.0.113.5" {
		t.Errorf("PublicIP = %q, want %q", got.PublicIP.ValueString(), "203.0.113.5")
	}
	if got.Status.ValueString() != "Enabled" {
		t.Errorf("Status = %q, want %q", got.Status.ValueString(), "Enabled")
	}
	if got.ZoneName.ValueString() != "YOW-1" {
		t.Errorf("ZoneName = %q, want %q", got.ZoneName.ValueString(), "YOW-1")
	}
	// vpc is write-only; must be preserved from state.
	if got.VPC.ValueString() != "my-vpc" {
		t.Errorf("VPC = %q, want preserved value %q", got.VPC.ValueString(), "my-vpc")
	}
}

func TestVPCVPNGatewayResource_readNotFound(t *testing.T) {
	svc := &fakeVPNGatewayService{gateways: []vpc.VPNGateway{}}
	resp := readVPNGateway(t, svc, "my-vpc", "missing-slug")
	if resp.Diagnostics.HasError() {
		t.Fatalf("read-not-found should not produce diagnostics: %v", resp.Diagnostics)
	}
	// RemoveResource sets the state to null.
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestVPCVPNGatewayResource_deleteHappyPath(t *testing.T) {
	svc := &fakeVPNGatewayService{}
	resp := deleteVPNGateway(t, svc, "my-vpc", "gw-slug-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "my-vpc/gw-slug-1" {
		t.Errorf("Delete called with %v, want [my-vpc/gw-slug-1]", svc.deleted)
	}
}

func TestVPCVPNGatewayResource_delete404IsNoOp(t *testing.T) {
	svc := &fakeVPNGatewayService{err: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := deleteVPNGateway(t, svc, "my-vpc", "gw-slug-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 on delete should be a no-op: %v", resp.Diagnostics)
	}
}
