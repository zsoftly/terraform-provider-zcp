package provider_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/ipaddress"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeIPAddressService satisfies ipAddressServiceIface.
type fakeIPAddressService struct {
	allocated *ipaddress.IPAddress
	ips       []ipaddress.IPAddress
	err       error
	released  []string
}

func (f *fakeIPAddressService) Allocate(_ context.Context, _ ipaddress.CreateRequest) (*ipaddress.IPAddress, error) {
	return f.allocated, f.err
}
func (f *fakeIPAddressService) List(_ context.Context, _, _, _ string) ([]ipaddress.IPAddress, error) {
	return f.ips, f.err
}
func (f *fakeIPAddressService) Release(_ context.Context, slug string) error {
	f.released = append(f.released, slug)
	return f.err
}

// ipAddressStateModel mirrors ipAddressResourceModel for state extraction in tests.
type ipAddressStateModel struct {
	ID           types.String   `tfsdk:"id"`
	Plan         types.String   `tfsdk:"plan"`
	BillingCycle types.String   `tfsdk:"billing_cycle"`
	VPC          types.String   `tfsdk:"vpc"`
	Network      types.String   `tfsdk:"network"`
	Project      types.String   `tfsdk:"project"`
	IPAddress    types.String   `tfsdk:"ip_address"`
	Type         types.String   `tfsdk:"type"`
	Timeouts     timeouts.Value `tfsdk:"timeouts"`
}

func ipAddressSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewIPAddressResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func ipAddressTFType(t *testing.T) tftypes.Type {
	t.Helper()
	return ipAddressSchema(t).Schema.Type().TerraformType(context.Background())
}

func createIPAddress(t *testing.T, svc *fakeIPAddressService, plan, billingCycle string) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewIPAddressResourceWithService(svc)
	schResp := ipAddressSchema(t)
	tfType := ipAddressTFType(t)
	planVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, nil),
		"plan":          tftypes.NewValue(tftypes.String, plan),
		"billing_cycle": tftypes.NewValue(tftypes.String, billingCycle),
		"vpc":           tftypes.NewValue(tftypes.String, nil),
		"network":       tftypes.NewValue(tftypes.String, nil),
		"project":       tftypes.NewValue(tftypes.String, nil),
		"ip_address":    tftypes.NewValue(tftypes.String, nil),
		"type":          tftypes.NewValue(tftypes.String, nil),
		"timeouts":      timeoutsNull(t, schResp),
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

// createIPAddressWithVPC is like createIPAddress but sets the vpc attribute,
// for exercising the VPC-named branch of the "no networks in vpc" error.
func createIPAddressWithVPC(t *testing.T, svc *fakeIPAddressService, plan, billingCycle, vpc string) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewIPAddressResourceWithService(svc)
	schResp := ipAddressSchema(t)
	tfType := ipAddressTFType(t)
	planVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, nil),
		"plan":          tftypes.NewValue(tftypes.String, plan),
		"billing_cycle": tftypes.NewValue(tftypes.String, billingCycle),
		"vpc":           tftypes.NewValue(tftypes.String, vpc),
		"network":       tftypes.NewValue(tftypes.String, nil),
		"project":       tftypes.NewValue(tftypes.String, nil),
		"ip_address":    tftypes.NewValue(tftypes.String, nil),
		"type":          tftypes.NewValue(tftypes.String, nil),
		"timeouts":      timeoutsNull(t, schResp),
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

func readIPAddress(t *testing.T, svc *fakeIPAddressService, slug string) resource.ReadResponse {
	t.Helper()
	var r resource.Resource
	if svc != nil {
		r = internalprovider.NewIPAddressResourceWithService(svc)
	} else {
		r = internalprovider.NewIPAddressResource()
	}
	schResp := ipAddressSchema(t)
	tfType := ipAddressTFType(t)
	stateVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, slug),
		"plan":          tftypes.NewValue(tftypes.String, "public-ip-1"),
		"billing_cycle": tftypes.NewValue(tftypes.String, "hourly"),
		"vpc":           tftypes.NewValue(tftypes.String, nil),
		"network":       tftypes.NewValue(tftypes.String, nil),
		"project":       tftypes.NewValue(tftypes.String, nil),
		"ip_address":    tftypes.NewValue(tftypes.String, "10.18.30.45"),
		"type":          tftypes.NewValue(tftypes.String, "Public"),
		"timeouts":      timeoutsNull(t, schResp),
	})
	readReq := resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), readReq, readResp)
	return *readResp
}

func deleteIPAddress(t *testing.T, svc *fakeIPAddressService, slug string) resource.DeleteResponse {
	t.Helper()
	r := internalprovider.NewIPAddressResourceWithService(svc)
	schResp := ipAddressSchema(t)
	tfType := ipAddressTFType(t)
	stateVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, slug),
		"plan":          tftypes.NewValue(tftypes.String, "public-ip-1"),
		"billing_cycle": tftypes.NewValue(tftypes.String, "hourly"),
		"vpc":           tftypes.NewValue(tftypes.String, nil),
		"network":       tftypes.NewValue(tftypes.String, nil),
		"project":       tftypes.NewValue(tftypes.String, nil),
		"ip_address":    tftypes.NewValue(tftypes.String, "10.18.30.45"),
		"type":          tftypes.NewValue(tftypes.String, "Public"),
		"timeouts":      timeoutsNull(t, schResp),
	})
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

func TestIPAddressResource_createHappyPath(t *testing.T) {
	svc := &fakeIPAddressService{
		allocated: &ipaddress.IPAddress{
			Slug:      "1036521143",
			IPAddress: "10.18.30.45",
			Type:      "Public",
		},
	}
	resp := createIPAddress(t, svc, "public-ip-1", "hourly")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got ipAddressStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "1036521143" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "1036521143")
	}
	if got.IPAddress.ValueString() != "10.18.30.45" {
		t.Errorf("IPAddress = %q, want %q", got.IPAddress.ValueString(), "10.18.30.45")
	}
	if got.Type.ValueString() != "Public" {
		t.Errorf("Type = %q, want %q", got.Type.ValueString(), "Public")
	}
}

func TestIPAddressResource_createServiceError(t *testing.T) {
	svc := &fakeIPAddressService{err: errors.New("quota exceeded")}
	resp := createIPAddress(t, svc, "public-ip-1", "hourly")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

// When Allocate fails because the VPC has no network yet, Create must replace
// the raw API error with a friendlier diagnostic naming the depends_on fix,
// while still surfacing the original API error text in the diagnostic detail.
func TestIPAddressResource_createNoNetworksInVPCReplacesError(t *testing.T) {
	origErr := "We cannot acquire IP Address when there are no networks in vpc."
	svc := &fakeIPAddressService{err: errors.New(origErr)}
	resp := createIPAddress(t, svc, "public-ip-1", "hourly")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when the VPC has no network yet")
	}
	found := false
	var detail string
	for _, d := range resp.Diagnostics {
		if d.Summary() == "VPC has no network yet" {
			found = true
			detail = d.Detail()
		}
	}
	if !found {
		t.Fatalf("expected a diagnostic summary %q, got: %v", "VPC has no network yet", resp.Diagnostics)
	}
	if !strings.Contains(detail, origErr) {
		t.Errorf("diagnostic detail = %q, want it to contain the original API error %q", detail, origErr)
	}
}

// When the vpc attribute is null (the API error can also arise when only
// `network` is set), the friendly message must not render an empty `VPC ""`.
func TestIPAddressResource_createNoNetworksInVPCNullVPCOmitsEmptyName(t *testing.T) {
	svc := &fakeIPAddressService{
		err: errors.New("We cannot acquire IP Address when there are no networks in vpc."),
	}
	resp := createIPAddress(t, svc, "public-ip-1", "hourly")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when the VPC has no network yet")
	}
	for _, d := range resp.Diagnostics {
		if d.Summary() != "VPC has no network yet" {
			continue
		}
		if strings.Contains(d.Detail(), `VPC ""`) {
			t.Errorf("diagnostic detail = %q, should not render an empty VPC name", d.Detail())
		}
	}
}

// When the vpc attribute is set, the friendly message names it.
func TestIPAddressResource_createNoNetworksInVPCNamesConfiguredVPC(t *testing.T) {
	svc := &fakeIPAddressService{
		err: errors.New("We cannot acquire IP Address when there are no networks in vpc."),
	}
	resp := createIPAddressWithVPC(t, svc, "public-ip-1", "hourly", "main-vpc")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when the VPC has no network yet")
	}
	found := false
	for _, d := range resp.Diagnostics {
		if d.Summary() == "VPC has no network yet" && strings.Contains(d.Detail(), `VPC "main-vpc"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the diagnostic to name VPC \"main-vpc\", got: %v", resp.Diagnostics)
	}
}

func TestIPAddressResource_readFound(t *testing.T) {
	svc := &fakeIPAddressService{
		ips: []ipaddress.IPAddress{
			{Slug: "1036521143", IPAddress: "10.18.30.45", Type: "Public"},
		},
	}
	resp := readIPAddress(t, svc, "1036521143")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got ipAddressStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.IPAddress.ValueString() != "10.18.30.45" {
		t.Errorf("IPAddress = %q, want %q", got.IPAddress.ValueString(), "10.18.30.45")
	}
	if got.Type.ValueString() != "Public" {
		t.Errorf("Type = %q, want %q", got.Type.ValueString(), "Public")
	}
	// Write-only fields must be preserved from state.
	if got.Plan.ValueString() != "public-ip-1" {
		t.Errorf("Plan = %q, want preserved value %q", got.Plan.ValueString(), "public-ip-1")
	}
	if got.BillingCycle.ValueString() != "hourly" {
		t.Errorf("BillingCycle = %q, want preserved value %q", got.BillingCycle.ValueString(), "hourly")
	}
}

func TestIPAddressResource_readNotFound(t *testing.T) {
	svc := &fakeIPAddressService{ips: []ipaddress.IPAddress{}}
	resp := readIPAddress(t, svc, "missing-slug")
	if resp.Diagnostics.HasError() {
		t.Fatalf("read-not-found should not produce diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestIPAddressResource_deleteHappyPath(t *testing.T) {
	svc := &fakeIPAddressService{}
	resp := deleteIPAddress(t, svc, "1036521143")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.released) != 1 || svc.released[0] != "1036521143" {
		t.Errorf("Release called with %v, want [1036521143]", svc.released)
	}
}

func TestIPAddressResource_delete404IsNoOp(t *testing.T) {
	svc := &fakeIPAddressService{err: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := deleteIPAddress(t, svc, "gone-slug")
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 on delete should be a no-op: %v", resp.Diagnostics)
	}
}
