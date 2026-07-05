package provider_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/ipaddress"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeIPAssociationService satisfies ipAssociationServiceIface.
type fakeIPAssociationService struct {
	enabled   []string // "<ip>-><vm>@<net>"
	disabled  []string
	ips       []ipaddress.IPAddress
	enableErr error
}

func (f *fakeIPAssociationService) Enable(_ context.Context, ip, vm, net string) error {
	f.enabled = append(f.enabled, ip+"->"+vm+"@"+net)
	return f.enableErr
}
func (f *fakeIPAssociationService) Disable(_ context.Context, ip string) error {
	f.disabled = append(f.disabled, ip)
	return nil
}
func (f *fakeIPAssociationService) List(_ context.Context) ([]ipaddress.IPAddress, error) {
	return f.ips, nil
}

func ipAssocSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewIPAssociationResource()
	var s resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &s)
	return s
}

func ipAssocValues(id string) map[string]tftypes.Value {
	idVal := tftypes.NewValue(tftypes.String, nil)
	if id != "" {
		idVal = tftypes.NewValue(tftypes.String, id)
	}
	return map[string]tftypes.Value{
		"id":              idVal,
		"ip_address":      tftypes.NewValue(tftypes.String, "206248159156"),
		"virtual_machine": tftypes.NewValue(tftypes.String, "qa-vm"),
		"network":         tftypes.NewValue(tftypes.String, "app-tier"),
	}
}

func TestIPAssociationResource_createEnablesStaticNAT(t *testing.T) {
	svc := &fakeIPAssociationService{}
	r := internalprovider.NewIPAssociationResourceWithService(svc)
	schResp := ipAssocSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	planVal := tftypes.NewValue(tfType, ipAssocValues(""))
	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)}}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: planVal}}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", createResp.Diagnostics)
	}
	if len(svc.enabled) != 1 || svc.enabled[0] != "206248159156->qa-vm@app-tier" {
		t.Errorf("Enable called with %v, want [206248159156->qa-vm@app-tier]", svc.enabled)
	}
	var got struct {
		ID             types.String `tfsdk:"id"`
		IPAddress      types.String `tfsdk:"ip_address"`
		VirtualMachine types.String `tfsdk:"virtual_machine"`
		Network        types.String `tfsdk:"network"`
	}
	if diags := createResp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "206248159156" {
		t.Errorf("ID = %q, want the IP slug", got.ID.ValueString())
	}
}

func TestIPAssociationResource_deleteDisassociates(t *testing.T) {
	svc := &fakeIPAssociationService{}
	r := internalprovider.NewIPAssociationResourceWithService(svc)
	schResp := ipAssocSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	stateVal := tftypes.NewValue(tfType, ipAssocValues("206248159156"))
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", deleteResp.Diagnostics)
	}
	if len(svc.disabled) != 1 || svc.disabled[0] != "206248159156" {
		t.Errorf("Disable called with %v, want [206248159156]", svc.disabled)
	}
}

func TestIPAssociationResource_readGoneWhenNoVM(t *testing.T) {
	// IP still exists but no VM attached → association removed out of band.
	svc := &fakeIPAssociationService{ips: []ipaddress.IPAddress{{Slug: "206248159156", VirtualMachineID: ""}}}
	r := internalprovider.NewIPAssociationResourceWithService(svc)
	schResp := ipAssocSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	stateVal := tftypes.NewValue(tfType, ipAssocValues("206248159156"))
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics)
	}
	if !readResp.State.Raw.IsNull() {
		t.Error("expected null state (association gone), got non-null")
	}
}
