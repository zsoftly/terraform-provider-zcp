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

// fakeVPNCustomerGatewayService satisfies vpnCustomerGatewayServiceIface.
type fakeVPNCustomerGatewayService struct {
	gateways []vpn.CustomerGateway
	created  *vpn.CustomerGateway
	updated  *vpn.CustomerGateway
	err      error
	deleted  []string
}

func (f *fakeVPNCustomerGatewayService) List(_ context.Context) ([]vpn.CustomerGateway, error) {
	return f.gateways, f.err
}
func (f *fakeVPNCustomerGatewayService) Create(_ context.Context, _ vpn.CustomerGatewayRequest) (*vpn.CustomerGateway, error) {
	return f.created, f.err
}
func (f *fakeVPNCustomerGatewayService) Update(_ context.Context, _ string, _ vpn.CustomerGatewayRequest) (*vpn.CustomerGateway, error) {
	return f.updated, f.err
}
func (f *fakeVPNCustomerGatewayService) Get(_ context.Context, slug string) (*vpn.CustomerGateway, error) {
	if f.err != nil {
		return nil, f.err
	}
	for i := range f.gateways {
		if f.gateways[i].Slug == slug {
			return &f.gateways[i], nil
		}
	}
	return nil, &apierrors.APIError{StatusCode: 404, Message: "not found"}
}
func (f *fakeVPNCustomerGatewayService) Delete(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	return f.err
}

// vpnCustomerGatewayStateModel mirrors vpnCustomerGatewayResourceModel for state extraction in tests.
type vpnCustomerGatewayStateModel struct {
	ID                 types.String   `tfsdk:"id"`
	Name               types.String   `tfsdk:"name"`
	Gateway            types.String   `tfsdk:"gateway"`
	CIDRList           types.String   `tfsdk:"cidr_list"`
	IPSecPSK           types.String   `tfsdk:"ipsec_psk"`
	IKEPolicy          types.String   `tfsdk:"ike_policy"`
	ESPPolicy          types.String   `tfsdk:"esp_policy"`
	IKELifetime        types.String   `tfsdk:"ike_lifetime"`
	ESPLifetime        types.String   `tfsdk:"esp_lifetime"`
	IKEEncryption      types.String   `tfsdk:"ike_encryption"`
	IKEHash            types.String   `tfsdk:"ike_hash"`
	IKEVersion         types.String   `tfsdk:"ike_version"`
	IKEDH              types.String   `tfsdk:"ike_dh"`
	ESPEncryption      types.String   `tfsdk:"esp_encryption"`
	ESPHash            types.String   `tfsdk:"esp_hash"`
	ESPDH              types.String   `tfsdk:"esp_dh"`
	ESPPFS             types.String   `tfsdk:"esp_pfs"`
	ForceEncapsulation types.Bool     `tfsdk:"force_encapsulation"`
	SplitConnections   types.Bool     `tfsdk:"split_connections"`
	DeadPeerDetection  types.Bool     `tfsdk:"dead_peer_detection"`
	CloudProvider      types.String   `tfsdk:"cloud_provider"`
	Region             types.String   `tfsdk:"region"`
	Project            types.String   `tfsdk:"project"`
	Timeouts           timeouts.Value `tfsdk:"timeouts"`
}

func vpnCGSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewVPNCustomerGatewayResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func vpnCGTFType(t *testing.T) tftypes.Type {
	t.Helper()
	return vpnCGSchema(t).Schema.Type().TerraformType(context.Background())
}

// vpnCGPlanValues builds a tftypes.Value map with sensible defaults for the plan.
// Override individual keys by mutating the returned map before calling tftypes.NewValue.
func vpnCGPlanValues(t *testing.T, schResp resource.SchemaResponse, overrides map[string]tftypes.Value) map[string]tftypes.Value {
	t.Helper()
	base := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, nil),
		"name":                tftypes.NewValue(tftypes.String, "remote-gw"),
		"gateway":             tftypes.NewValue(tftypes.String, "203.0.113.1"),
		"cidr_list":           tftypes.NewValue(tftypes.String, "192.168.1.0/24"),
		"ipsec_psk":           tftypes.NewValue(tftypes.String, "s3cr3t"),
		"ike_policy":          tftypes.NewValue(tftypes.String, "aes128-sha1-dh5"),
		"esp_policy":          tftypes.NewValue(tftypes.String, "aes128-sha1"),
		"ike_lifetime":        tftypes.NewValue(tftypes.String, nil),
		"esp_lifetime":        tftypes.NewValue(tftypes.String, nil),
		"ike_encryption":      tftypes.NewValue(tftypes.String, nil),
		"ike_hash":            tftypes.NewValue(tftypes.String, nil),
		"ike_version":         tftypes.NewValue(tftypes.String, nil),
		"ike_dh":              tftypes.NewValue(tftypes.String, nil),
		"esp_encryption":      tftypes.NewValue(tftypes.String, nil),
		"esp_hash":            tftypes.NewValue(tftypes.String, nil),
		"esp_dh":              tftypes.NewValue(tftypes.String, nil),
		"esp_pfs":             tftypes.NewValue(tftypes.String, nil),
		"force_encapsulation": tftypes.NewValue(tftypes.Bool, false),
		"split_connections":   tftypes.NewValue(tftypes.Bool, false),
		"dead_peer_detection": tftypes.NewValue(tftypes.Bool, false),
		"cloud_provider":      tftypes.NewValue(tftypes.String, "nimbo"),
		"region":              tftypes.NewValue(tftypes.String, "yow-1"),
		"project":             tftypes.NewValue(tftypes.String, nil),
		"timeouts":            timeoutsNull(t, schResp),
	}
	for k, v := range overrides {
		base[k] = v
	}
	return base
}

func createVPNCG(t *testing.T, svc *fakeVPNCustomerGatewayService) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewVPNCustomerGatewayResourceWithService(svc)
	schResp := vpnCGSchema(t)
	tfType := vpnCGTFType(t)
	planVal := tftypes.NewValue(tfType, vpnCGPlanValues(t, schResp, nil))
	createReq := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: planVal},
	}
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)},
	}
	r.Create(context.Background(), createReq, createResp)
	return *createResp
}

func readVPNCG(t *testing.T, svc *fakeVPNCustomerGatewayService, slug string) resource.ReadResponse {
	t.Helper()
	var r resource.Resource
	if svc != nil {
		r = internalprovider.NewVPNCustomerGatewayResourceWithService(svc)
	} else {
		r = internalprovider.NewVPNCustomerGatewayResource()
	}
	schResp := vpnCGSchema(t)
	tfType := vpnCGTFType(t)
	stateVal := tftypes.NewValue(tfType, vpnCGPlanValues(t, schResp, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, slug),
	}))
	readReq := resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), readReq, readResp)
	return *readResp
}

func deleteVPNCG(t *testing.T, svc *fakeVPNCustomerGatewayService, slug string) resource.DeleteResponse {
	t.Helper()
	r := internalprovider.NewVPNCustomerGatewayResourceWithService(svc)
	schResp := vpnCGSchema(t)
	tfType := vpnCGTFType(t)
	stateVal := tftypes.NewValue(tfType, vpnCGPlanValues(t, schResp, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, slug),
	}))
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

func TestVPNCustomerGatewayResource_createHappyPath(t *testing.T) {
	svc := &fakeVPNCustomerGatewayService{
		created: &vpn.CustomerGateway{
			Slug:      "gw-1",
			Name:      "remote-gw",
			Gateway:   "203.0.113.1",
			CIDRList:  "192.168.1.0/24",
			IKEPolicy: "aes128-sha1-dh5",
			ESPPolicy: "aes128-sha1",
		},
	}
	resp := createVPNCG(t, svc)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got vpnCustomerGatewayStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "gw-1" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "gw-1")
	}
	if got.Name.ValueString() != "remote-gw" {
		t.Errorf("Name = %q, want %q", got.Name.ValueString(), "remote-gw")
	}
	if got.Gateway.ValueString() != "203.0.113.1" {
		t.Errorf("Gateway = %q, want %q", got.Gateway.ValueString(), "203.0.113.1")
	}
	if got.CIDRList.ValueString() != "192.168.1.0/24" {
		t.Errorf("CIDRList = %q, want %q", got.CIDRList.ValueString(), "192.168.1.0/24")
	}
	// ipsec_psk is write-only — must be preserved from the plan.
	if got.IPSecPSK.ValueString() != "s3cr3t" {
		t.Errorf("IPSecPSK = %q, want preserved write-only value %q", got.IPSecPSK.ValueString(), "s3cr3t")
	}
}

func TestVPNCustomerGatewayResource_createServiceError(t *testing.T) {
	svc := &fakeVPNCustomerGatewayService{err: errors.New("quota exceeded")}
	resp := createVPNCG(t, svc)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func TestVPNCustomerGatewayResource_readFound(t *testing.T) {
	svc := &fakeVPNCustomerGatewayService{
		gateways: []vpn.CustomerGateway{
			{
				Slug:      "gw-1",
				Name:      "remote-gw",
				Gateway:   "203.0.113.1",
				CIDRList:  "192.168.1.0/24",
				IKEPolicy: "aes128-sha1-dh5",
				ESPPolicy: "aes128-sha1",
			},
		},
	}
	resp := readVPNCG(t, svc, "gw-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got vpnCustomerGatewayStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Name.ValueString() != "remote-gw" {
		t.Errorf("Name = %q, want %q", got.Name.ValueString(), "remote-gw")
	}
	if got.IKEPolicy.ValueString() != "aes128-sha1-dh5" {
		t.Errorf("IKEPolicy = %q, want %q", got.IKEPolicy.ValueString(), "aes128-sha1-dh5")
	}
	// ipsec_psk is write-only — must be preserved from state, not wiped.
	if got.IPSecPSK.ValueString() != "s3cr3t" {
		t.Errorf("IPSecPSK = %q, want preserved state value %q", got.IPSecPSK.ValueString(), "s3cr3t")
	}
	// cloud_provider is write-only — must be preserved from state.
	if got.CloudProvider.ValueString() != "nimbo" {
		t.Errorf("CloudProvider = %q, want preserved state value %q", got.CloudProvider.ValueString(), "nimbo")
	}
}

func TestVPNCustomerGatewayResource_readNotFound(t *testing.T) {
	svc := &fakeVPNCustomerGatewayService{gateways: []vpn.CustomerGateway{}}
	resp := readVPNCG(t, svc, "missing-slug")
	if resp.Diagnostics.HasError() {
		t.Fatalf("read-not-found should not produce diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestVPNCustomerGatewayResource_deleteHappyPath(t *testing.T) {
	svc := &fakeVPNCustomerGatewayService{}
	resp := deleteVPNCG(t, svc, "gw-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "gw-1" {
		t.Errorf("Delete called with %v, want [gw-1]", svc.deleted)
	}
}

func TestVPNCustomerGatewayResource_delete404IsNoOp(t *testing.T) {
	svc := &fakeVPNCustomerGatewayService{err: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := deleteVPNCG(t, svc, "gone-slug")
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 on delete should be a no-op: %v", resp.Diagnostics)
	}
}

func TestVPNCustomerGatewayResource_updateName(t *testing.T) {
	svc := &fakeVPNCustomerGatewayService{
		updated: &vpn.CustomerGateway{
			Slug:      "gw-1",
			Name:      "remote-gw-renamed",
			Gateway:   "203.0.113.1",
			CIDRList:  "192.168.1.0/24",
			IKEPolicy: "aes128-sha1-dh5",
			ESPPolicy: "aes128-sha1",
		},
	}
	r := internalprovider.NewVPNCustomerGatewayResourceWithService(svc)
	schResp := vpnCGSchema(t)
	tfType := vpnCGTFType(t)

	existingState := tftypes.NewValue(tfType, vpnCGPlanValues(t, schResp, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "gw-1"),
		"name":      tftypes.NewValue(tftypes.String, "remote-gw"),
		"ipsec_psk": tftypes.NewValue(tftypes.String, "s3cr3t"),
	}))
	newPlan := tftypes.NewValue(tfType, vpnCGPlanValues(t, schResp, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "gw-1"),
		"name":      tftypes.NewValue(tftypes.String, "remote-gw-renamed"),
		"ipsec_psk": tftypes.NewValue(tftypes.String, "s3cr3t"),
	}))

	updateReq := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schResp.Schema, Raw: newPlan},
		State: tfsdk.State{Schema: schResp.Schema, Raw: existingState},
	}
	updateResp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: existingState},
	}
	r.Update(context.Background(), updateReq, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", updateResp.Diagnostics)
	}
	var got vpnCustomerGatewayStateModel
	if diags := updateResp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Name.ValueString() != "remote-gw-renamed" {
		t.Errorf("Name = %q, want %q", got.Name.ValueString(), "remote-gw-renamed")
	}
	// Write-only and immutable fields must be preserved from state.
	if got.IPSecPSK.ValueString() != "s3cr3t" {
		t.Errorf("IPSecPSK = %q, want preserved state value %q", got.IPSecPSK.ValueString(), "s3cr3t")
	}
	if got.Gateway.ValueString() != "203.0.113.1" {
		t.Errorf("Gateway = %q, want preserved state value %q", got.Gateway.ValueString(), "203.0.113.1")
	}
	if got.CIDRList.ValueString() != "192.168.1.0/24" {
		t.Errorf("CIDRList = %q, want preserved state value %q", got.CIDRList.ValueString(), "192.168.1.0/24")
	}
	if got.CloudProvider.ValueString() != "nimbo" {
		t.Errorf("CloudProvider = %q, want preserved state value %q", got.CloudProvider.ValueString(), "nimbo")
	}
}
