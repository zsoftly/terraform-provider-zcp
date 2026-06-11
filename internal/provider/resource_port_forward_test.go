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
	"github.com/zsoftly/zcp-cli/pkg/api/portforward"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakePortForwardService satisfies portForwardServiceIface.
type fakePortForwardService struct {
	rules   []portforward.PortForwardRule
	created *portforward.PortForwardRule
	err     error
	deleted []string
}

func (f *fakePortForwardService) List(_ context.Context, _ string) ([]portforward.PortForwardRule, error) {
	return f.rules, f.err
}
func (f *fakePortForwardService) Create(_ context.Context, _ string, _ portforward.CreateRequest) (*portforward.PortForwardRule, error) {
	return f.created, f.err
}
func (f *fakePortForwardService) Delete(_ context.Context, _ string, ruleID string) error {
	f.deleted = append(f.deleted, ruleID)
	return f.err
}

// portForwardStateModel mirrors portForwardResourceModel for state extraction in tests.
type portForwardStateModel struct {
	ID               types.String   `tfsdk:"id"`
	IPAddress        types.String   `tfsdk:"ip_address"`
	Protocol         types.String   `tfsdk:"protocol"`
	PublicStartPort  types.String   `tfsdk:"public_start_port"`
	PublicEndPort    types.String   `tfsdk:"public_end_port"`
	PrivateStartPort types.String   `tfsdk:"private_start_port"`
	PrivateEndPort   types.String   `tfsdk:"private_end_port"`
	VirtualMachine   types.String   `tfsdk:"virtual_machine"`
	State            types.String   `tfsdk:"state"`
	Timeouts         timeouts.Value `tfsdk:"timeouts"`
}

func portForwardSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewPortForwardResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func portForwardTFType(t *testing.T) tftypes.Type {
	t.Helper()
	s := portForwardSchema(t)
	return s.Schema.Type().TerraformType(context.Background())
}

// createPortForward calls Create on a resource wired with svc.
func createPortForward(t *testing.T, svc *fakePortForwardService) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewPortForwardResourceWithService(svc)
	schResp := portForwardSchema(t)
	tfType := portForwardTFType(t)
	planVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                 tftypes.NewValue(tftypes.String, nil),
		"ip_address":         tftypes.NewValue(tftypes.String, "1036521143"),
		"protocol":           tftypes.NewValue(tftypes.String, "tcp"),
		"public_start_port":  tftypes.NewValue(tftypes.String, "80"),
		"public_end_port":    tftypes.NewValue(tftypes.String, nil),
		"private_start_port": tftypes.NewValue(tftypes.String, "8080"),
		"private_end_port":   tftypes.NewValue(tftypes.String, nil),
		"virtual_machine":    tftypes.NewValue(tftypes.String, "my-vm"),
		"state":              tftypes.NewValue(tftypes.String, nil),
		"timeouts":           timeoutsNull(t, schResp),
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

// readPortForward calls Read on a resource wired with svc using the given rule ID.
func readPortForward(t *testing.T, svc *fakePortForwardService, ruleID string) resource.ReadResponse {
	t.Helper()
	r := internalprovider.NewPortForwardResourceWithService(svc)
	schResp := portForwardSchema(t)
	tfType := portForwardTFType(t)
	stateVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                 tftypes.NewValue(tftypes.String, ruleID),
		"ip_address":         tftypes.NewValue(tftypes.String, "1036521143"),
		"protocol":           tftypes.NewValue(tftypes.String, "tcp"),
		"public_start_port":  tftypes.NewValue(tftypes.String, "80"),
		"public_end_port":    tftypes.NewValue(tftypes.String, nil),
		"private_start_port": tftypes.NewValue(tftypes.String, "8080"),
		"private_end_port":   tftypes.NewValue(tftypes.String, nil),
		"virtual_machine":    tftypes.NewValue(tftypes.String, "my-vm"),
		"state":              tftypes.NewValue(tftypes.String, "active"),
		"timeouts":           timeoutsNull(t, schResp),
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

// deletePortForward calls Delete on a resource wired with svc.
func deletePortForward(t *testing.T, svc *fakePortForwardService, ruleID string) resource.DeleteResponse {
	t.Helper()
	r := internalprovider.NewPortForwardResourceWithService(svc)
	schResp := portForwardSchema(t)
	tfType := portForwardTFType(t)
	stateVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                 tftypes.NewValue(tftypes.String, ruleID),
		"ip_address":         tftypes.NewValue(tftypes.String, "1036521143"),
		"protocol":           tftypes.NewValue(tftypes.String, "tcp"),
		"public_start_port":  tftypes.NewValue(tftypes.String, "80"),
		"public_end_port":    tftypes.NewValue(tftypes.String, nil),
		"private_start_port": tftypes.NewValue(tftypes.String, "8080"),
		"private_end_port":   tftypes.NewValue(tftypes.String, nil),
		"virtual_machine":    tftypes.NewValue(tftypes.String, "my-vm"),
		"state":              tftypes.NewValue(tftypes.String, "active"),
		"timeouts":           timeoutsNull(t, schResp),
	})
	deleteReq := resource.DeleteRequest{
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

func TestPortForwardResource_createHappyPath(t *testing.T) {
	svc := &fakePortForwardService{
		created: &portforward.PortForwardRule{
			ID:               "pf-uuid-1",
			Protocol:         "tcp",
			PublicStartPort:  "80",
			PrivateStartPort: "8080",
			State:            "active",
		},
	}
	resp := createPortForward(t, svc)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got portForwardStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "pf-uuid-1" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "pf-uuid-1")
	}
	if got.State.ValueString() != "active" {
		t.Errorf("State = %q, want %q", got.State.ValueString(), "active")
	}
	if got.VirtualMachine.ValueString() != "my-vm" {
		t.Errorf("VirtualMachine = %q, want %q", got.VirtualMachine.ValueString(), "my-vm")
	}
}

func TestPortForwardResource_createServiceError(t *testing.T) {
	svc := &fakePortForwardService{err: errors.New("quota exceeded")}
	resp := createPortForward(t, svc)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func TestPortForwardResource_readFound(t *testing.T) {
	svc := &fakePortForwardService{
		rules: []portforward.PortForwardRule{
			{
				ID:               "pf-uuid-1",
				Protocol:         "tcp",
				PublicStartPort:  "80",
				PrivateStartPort: "8080",
				State:            "active",
			},
		},
	}
	resp := readPortForward(t, svc, "pf-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got portForwardStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "pf-uuid-1" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "pf-uuid-1")
	}
	// virtual_machine is write-only; preserved from state.
	if got.VirtualMachine.ValueString() != "my-vm" {
		t.Errorf("VirtualMachine = %q, want %q (should be preserved from state)", got.VirtualMachine.ValueString(), "my-vm")
	}
}

func TestPortForwardResource_readNotFound(t *testing.T) {
	svc := &fakePortForwardService{rules: []portforward.PortForwardRule{}}
	resp := readPortForward(t, svc, "pf-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("read-not-found should not produce diagnostics: %v", resp.Diagnostics)
	}
	// RemoveResource sets the state to null.
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestPortForwardResource_deleteHappyPath(t *testing.T) {
	svc := &fakePortForwardService{}
	resp := deletePortForward(t, svc, "pf-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "pf-uuid-1" {
		t.Errorf("Delete called with %v, want [pf-uuid-1]", svc.deleted)
	}
}

func TestPortForwardResource_delete404IsNoOp(t *testing.T) {
	svc := &fakePortForwardService{err: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := deletePortForward(t, svc, "pf-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 on delete should be a no-op: %v", resp.Diagnostics)
	}
}
