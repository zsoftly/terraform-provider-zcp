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

// fakeVPCService satisfies vpcServiceIface.
type fakeVPCService struct {
	vpcs    []vpc.VPC
	created *vpc.VPC
	updated *vpc.VPC
	err     error
	deleted []string
}

func (f *fakeVPCService) List(_ context.Context, _ string) ([]vpc.VPC, error) {
	return f.vpcs, f.err
}
func (f *fakeVPCService) Get(_ context.Context, slug string) (*vpc.VPC, error) {
	for i := range f.vpcs {
		if f.vpcs[i].Slug == slug {
			return &f.vpcs[i], f.err
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	return nil, &apierrors.APIError{StatusCode: 404, Message: "not found"}
}
func (f *fakeVPCService) Create(_ context.Context, _ vpc.CreateRequest) (*vpc.VPC, error) {
	return f.created, f.err
}
func (f *fakeVPCService) Update(_ context.Context, _ string, _ vpc.UpdateRequest) (*vpc.VPC, error) {
	return f.updated, f.err
}
func (f *fakeVPCService) Delete(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	return f.err
}

// vpcStateModel mirrors vpcResourceModel for state extraction in tests.
type vpcStateModel struct {
	ID              types.String   `tfsdk:"id"`
	Name            types.String   `tfsdk:"name"`
	CloudProvider   types.String   `tfsdk:"cloud_provider"`
	Region          types.String   `tfsdk:"region"`
	CIDR            types.String   `tfsdk:"cidr"`
	Size            types.String   `tfsdk:"size"`
	Project         types.String   `tfsdk:"project"`
	Description     types.String   `tfsdk:"description"`
	VPCType         types.String   `tfsdk:"type"`
	BillingCycle    types.String   `tfsdk:"billing_cycle"`
	Plan            types.String   `tfsdk:"plan"`
	StorageCategory types.String   `tfsdk:"storage_category"`
	Status          types.String   `tfsdk:"status"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
}

func vpcSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewVPCResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func vpcTFType(t *testing.T) tftypes.Type {
	t.Helper()
	return vpcSchema(t).Schema.Type().TerraformType(context.Background())
}

func createVPC(t *testing.T, svc *fakeVPCService, name, region, provider, cidr, size string) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewVPCResourceWithService(svc)
	schResp := vpcSchema(t)
	tfType := vpcTFType(t)
	planVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, nil),
		"name":             tftypes.NewValue(tftypes.String, name),
		"cloud_provider":   tftypes.NewValue(tftypes.String, provider),
		"region":           tftypes.NewValue(tftypes.String, region),
		"cidr":             tftypes.NewValue(tftypes.String, cidr),
		"size":             tftypes.NewValue(tftypes.String, size),
		"project":          tftypes.NewValue(tftypes.String, nil),
		"description":      tftypes.NewValue(tftypes.String, nil),
		"type":             tftypes.NewValue(tftypes.String, nil),
		"billing_cycle":    tftypes.NewValue(tftypes.String, nil),
		"plan":             tftypes.NewValue(tftypes.String, nil),
		"storage_category": tftypes.NewValue(tftypes.String, nil),
		"status":           tftypes.NewValue(tftypes.String, nil),
		"timeouts":         timeoutsNull(t, schResp),
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

func readVPC(t *testing.T, svc *fakeVPCService, slug string) resource.ReadResponse {
	t.Helper()
	var r resource.Resource
	if svc != nil {
		r = internalprovider.NewVPCResourceWithService(svc)
	} else {
		r = internalprovider.NewVPCResource()
	}
	schResp := vpcSchema(t)
	tfType := vpcTFType(t)
	stateVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, slug),
		"name":             tftypes.NewValue(tftypes.String, "testvpc"),
		"cloud_provider":   tftypes.NewValue(tftypes.String, "nimbo"),
		"region":           tftypes.NewValue(tftypes.String, "yow-1"),
		"cidr":             tftypes.NewValue(tftypes.String, "10.1.0.1"),
		"size":             tftypes.NewValue(tftypes.String, "24"),
		"project":          tftypes.NewValue(tftypes.String, nil),
		"description":      tftypes.NewValue(tftypes.String, nil),
		"type":             tftypes.NewValue(tftypes.String, nil),
		"billing_cycle":    tftypes.NewValue(tftypes.String, nil),
		"plan":             tftypes.NewValue(tftypes.String, nil),
		"storage_category": tftypes.NewValue(tftypes.String, nil),
		"status":           tftypes.NewValue(tftypes.String, "Running"),
		"timeouts":         timeoutsNull(t, schResp),
	})
	readReq := resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), readReq, readResp)
	return *readResp
}

func deleteVPC(t *testing.T, svc *fakeVPCService, slug string) resource.DeleteResponse {
	t.Helper()
	r := internalprovider.NewVPCResourceWithService(svc)
	schResp := vpcSchema(t)
	tfType := vpcTFType(t)
	stateVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, slug),
		"name":             tftypes.NewValue(tftypes.String, "testvpc"),
		"cloud_provider":   tftypes.NewValue(tftypes.String, "nimbo"),
		"region":           tftypes.NewValue(tftypes.String, "yow-1"),
		"cidr":             tftypes.NewValue(tftypes.String, "10.1.0.1"),
		"size":             tftypes.NewValue(tftypes.String, "24"),
		"project":          tftypes.NewValue(tftypes.String, nil),
		"description":      tftypes.NewValue(tftypes.String, nil),
		"type":             tftypes.NewValue(tftypes.String, nil),
		"billing_cycle":    tftypes.NewValue(tftypes.String, nil),
		"plan":             tftypes.NewValue(tftypes.String, nil),
		"storage_category": tftypes.NewValue(tftypes.String, nil),
		"status":           tftypes.NewValue(tftypes.String, "Running"),
		"timeouts":         timeoutsNull(t, schResp),
	})
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

func TestVPCResource_createHappyPath(t *testing.T) {
	svc := &fakeVPCService{
		created: &vpc.VPC{
			Slug:   "testvpc-abc123",
			Name:   "testvpc",
			Status: "Running",
			CIDR:   "10.0.0.0/22",
		},
	}
	resp := createVPC(t, svc, "testvpc", "yow", "cloudstack", "10.0.0.0/22", "small")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got vpcStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "testvpc-abc123" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "testvpc-abc123")
	}
	if got.Status.ValueString() != "Running" {
		t.Errorf("Status = %q, want %q", got.Status.ValueString(), "Running")
	}
}

func TestVPCResource_createServiceError(t *testing.T) {
	svc := &fakeVPCService{err: errors.New("quota exceeded")}
	resp := createVPC(t, svc, "testvpc", "yow", "cloudstack", "10.0.0.0/22", "small")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func TestVPCResource_readFound(t *testing.T) {
	svc := &fakeVPCService{
		vpcs: []vpc.VPC{
			{Slug: "testvpc-abc123", Name: "testvpc", Status: "Running", CIDR: "10.0.0.0/22"},
		},
	}
	resp := readVPC(t, svc, "testvpc-abc123")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got vpcStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Status.ValueString() != "Running" {
		t.Errorf("Status = %q, want %q", got.Status.ValueString(), "Running")
	}
	// Write-only fields must be preserved from state.
	if got.Size.ValueString() != "24" {
		t.Errorf("Size = %q, want preserved value %q", got.Size.ValueString(), "24")
	}
}

func TestVPCResource_readNotFound(t *testing.T) {
	svc := &fakeVPCService{vpcs: []vpc.VPC{}}
	resp := readVPC(t, svc, "missing-slug")
	if resp.Diagnostics.HasError() {
		t.Fatalf("read-not-found should not produce diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestVPCResource_readWithoutConfigure(t *testing.T) {
	resp := readVPC(t, nil, "any-slug")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when svc is nil, got none")
	}
}

func TestVPCResource_deleteHappyPath(t *testing.T) {
	svc := &fakeVPCService{}
	resp := deleteVPC(t, svc, "testvpc-abc123")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "testvpc-abc123" {
		t.Errorf("Delete called with %v, want [testvpc-abc123]", svc.deleted)
	}
}

func TestVPCResource_delete404IsNoOp(t *testing.T) {
	svc := &fakeVPCService{err: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := deleteVPC(t, svc, "gone-slug")
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 on delete should be a no-op: %v", resp.Diagnostics)
	}
}

func TestVPCResource_updateNameDescription(t *testing.T) {
	svc := &fakeVPCService{
		updated: &vpc.VPC{Slug: "testvpc-abc123", Name: "new-name", Status: "Running"},
	}
	r := internalprovider.NewVPCResourceWithService(svc)
	schResp := vpcSchema(t)
	tfType := vpcTFType(t)

	existingState := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "testvpc-abc123"),
		"name":             tftypes.NewValue(tftypes.String, "old-name"),
		"cloud_provider":   tftypes.NewValue(tftypes.String, "nimbo"),
		"region":           tftypes.NewValue(tftypes.String, "yow-1"),
		"cidr":             tftypes.NewValue(tftypes.String, "10.1.0.1"),
		"size":             tftypes.NewValue(tftypes.String, "24"),
		"project":          tftypes.NewValue(tftypes.String, nil),
		"description":      tftypes.NewValue(tftypes.String, "old-desc"),
		"type":             tftypes.NewValue(tftypes.String, nil),
		"billing_cycle":    tftypes.NewValue(tftypes.String, nil),
		"plan":             tftypes.NewValue(tftypes.String, nil),
		"storage_category": tftypes.NewValue(tftypes.String, nil),
		"status":           tftypes.NewValue(tftypes.String, "Running"),
		"timeouts":         timeoutsNull(t, schResp),
	})
	newPlan := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "testvpc-abc123"),
		"name":             tftypes.NewValue(tftypes.String, "new-name"),
		"cloud_provider":   tftypes.NewValue(tftypes.String, "nimbo"),
		"region":           tftypes.NewValue(tftypes.String, "yow-1"),
		"cidr":             tftypes.NewValue(tftypes.String, "10.1.0.1"),
		"size":             tftypes.NewValue(tftypes.String, "24"),
		"project":          tftypes.NewValue(tftypes.String, nil),
		"description":      tftypes.NewValue(tftypes.String, "new-desc"),
		"type":             tftypes.NewValue(tftypes.String, nil),
		"billing_cycle":    tftypes.NewValue(tftypes.String, nil),
		"plan":             tftypes.NewValue(tftypes.String, nil),
		"storage_category": tftypes.NewValue(tftypes.String, nil),
		"status":           tftypes.NewValue(tftypes.String, nil),
		"timeouts":         timeoutsNull(t, schResp),
	})
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
	var got vpcStateModel
	if diags := updateResp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Name.ValueString() != "new-name" {
		t.Errorf("Name = %q, want %q", got.Name.ValueString(), "new-name")
	}
	if got.Size.ValueString() != "24" {
		t.Errorf("Size = %q, want preserved value %q", got.Size.ValueString(), "24")
	}
	if got.CIDR.ValueString() != "10.1.0.1" {
		t.Errorf("CIDR = %q, want preserved value %q", got.CIDR.ValueString(), "10.1.0.1")
	}
}

// Same omitempty guard as network: clearing description must preserve prior state value.
func TestVPCResource_updateClearsDescriptionWhenEmpty(t *testing.T) {
	svc := &fakeVPCService{
		updated: &vpc.VPC{Slug: "testvpc-abc123", Name: "same-name", Status: "Running"},
	}
	r := internalprovider.NewVPCResourceWithService(svc)
	schResp := vpcSchema(t)
	tfType := vpcTFType(t)

	existingState := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "testvpc-abc123"),
		"name":             tftypes.NewValue(tftypes.String, "same-name"),
		"cloud_provider":   tftypes.NewValue(tftypes.String, "nimbo"),
		"region":           tftypes.NewValue(tftypes.String, "yow-1"),
		"cidr":             tftypes.NewValue(tftypes.String, "10.1.0.1"),
		"size":             tftypes.NewValue(tftypes.String, "24"),
		"project":          tftypes.NewValue(tftypes.String, nil),
		"description":      tftypes.NewValue(tftypes.String, "old-desc"),
		"type":             tftypes.NewValue(tftypes.String, nil),
		"billing_cycle":    tftypes.NewValue(tftypes.String, nil),
		"plan":             tftypes.NewValue(tftypes.String, nil),
		"storage_category": tftypes.NewValue(tftypes.String, nil),
		"status":           tftypes.NewValue(tftypes.String, "Running"),
		"timeouts":         timeoutsNull(t, schResp),
	})
	emptyDescPlan := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "testvpc-abc123"),
		"name":             tftypes.NewValue(tftypes.String, "same-name"),
		"cloud_provider":   tftypes.NewValue(tftypes.String, "nimbo"),
		"region":           tftypes.NewValue(tftypes.String, "yow-1"),
		"cidr":             tftypes.NewValue(tftypes.String, "10.1.0.1"),
		"size":             tftypes.NewValue(tftypes.String, "24"),
		"project":          tftypes.NewValue(tftypes.String, nil),
		"description":      tftypes.NewValue(tftypes.String, ""),
		"type":             tftypes.NewValue(tftypes.String, nil),
		"billing_cycle":    tftypes.NewValue(tftypes.String, nil),
		"plan":             tftypes.NewValue(tftypes.String, nil),
		"storage_category": tftypes.NewValue(tftypes.String, nil),
		"status":           tftypes.NewValue(tftypes.String, nil),
		"timeouts":         timeoutsNull(t, schResp),
	})
	updateReq := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schResp.Schema, Raw: emptyDescPlan},
		State: tfsdk.State{Schema: schResp.Schema, Raw: existingState},
	}
	updateResp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: existingState},
	}
	r.Update(context.Background(), updateReq, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", updateResp.Diagnostics)
	}
	var got vpcStateModel
	if diags := updateResp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	// Empty description must flow through to state — not silently overridden with the old value.
	if got.Description.ValueString() != "" {
		t.Errorf("Description = %q, want %q (empty description should be honoured)", got.Description.ValueString(), "")
	}
}
