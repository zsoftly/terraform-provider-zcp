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
	"github.com/zsoftly/zcp-cli/pkg/api/affinitygroup"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeAffinityGroupService satisfies affinityGroupServiceIface.
type fakeAffinityGroupService struct {
	groups    []affinitygroup.AffinityGroup
	created   *affinitygroup.AffinityGroup
	createReq affinitygroup.CreateRequest
	err       error
	deleted   []string
}

func (f *fakeAffinityGroupService) List(_ context.Context, _, _ string) ([]affinitygroup.AffinityGroup, error) {
	return f.groups, f.err
}
func (f *fakeAffinityGroupService) Create(_ context.Context, req affinitygroup.CreateRequest) (*affinitygroup.AffinityGroup, error) {
	f.createReq = req
	return f.created, f.err
}
func (f *fakeAffinityGroupService) Delete(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	return f.err
}

// affinityGroupStateModel mirrors affinityGroupResourceModel for state extraction in tests.
type affinityGroupStateModel struct {
	ID            types.String   `tfsdk:"id"`
	Name          types.String   `tfsdk:"name"`
	Type          types.String   `tfsdk:"type"`
	Description   types.String   `tfsdk:"description"`
	CloudProvider types.String   `tfsdk:"cloud_provider"`
	Region        types.String   `tfsdk:"region"`
	Project       types.String   `tfsdk:"project"`
	State         types.String   `tfsdk:"state"`
	Timeouts      timeouts.Value `tfsdk:"timeouts"`
}

func affinityGroupSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewAffinityGroupResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func affinityGroupRaw(t *testing.T, schResp resource.SchemaResponse, id, name, groupType, state string) tftypes.Value {
	t.Helper()
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	str := func(v string) tftypes.Value {
		if v == "" {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, v)
	}
	return tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             str(id),
		"name":           str(name),
		"type":           str(groupType),
		"description":    tftypes.NewValue(tftypes.String, nil),
		"cloud_provider": str("zsoftly"),
		"region":         str("yow-1"),
		"project":        tftypes.NewValue(tftypes.String, nil),
		"state":          str(state),
		"timeouts":       timeoutsNull(t, schResp),
	})
}

func createAffinityGroup(t *testing.T, svc *fakeAffinityGroupService, name, groupType string) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewAffinityGroupResourceWithService(svc)
	schResp := affinityGroupSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	planVal := affinityGroupRaw(t, schResp, "", name, groupType, "")
	createReq := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: planVal},
	}
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)},
	}
	r.Create(context.Background(), createReq, createResp)
	return *createResp
}

func readAffinityGroup(t *testing.T, svc *fakeAffinityGroupService, id string) resource.ReadResponse {
	t.Helper()
	r := internalprovider.NewAffinityGroupResourceWithService(svc)
	schResp := affinityGroupSchema(t)
	stateVal := affinityGroupRaw(t, schResp, id, "web-anti", "host anti-affinity", "Active")
	readReq := resource.ReadRequest{
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	readResp := &resource.ReadResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	r.Read(context.Background(), readReq, readResp)
	return *readResp
}

func deleteAffinityGroup(t *testing.T, svc *fakeAffinityGroupService, id string) resource.DeleteResponse {
	t.Helper()
	r := internalprovider.NewAffinityGroupResourceWithService(svc)
	schResp := affinityGroupSchema(t)
	stateVal := affinityGroupRaw(t, schResp, id, "web-anti", "host anti-affinity", "Active")
	deleteReq := resource.DeleteRequest{
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

func TestAffinityGroupResource_createHappyPath(t *testing.T) {
	svc := &fakeAffinityGroupService{
		created: &affinitygroup.AffinityGroup{
			ID:    "ag-uuid-1",
			Slug:  "web-anti-x1y2",
			Name:  "web-anti",
			Type:  "host anti-affinity",
			State: "Active",
		},
	}
	resp := createAffinityGroup(t, svc, "web-anti", "host anti-affinity")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got affinityGroupStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "web-anti-x1y2" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "web-anti-x1y2")
	}
	if svc.createReq.Type != "host anti-affinity" {
		t.Errorf("create request type = %q, want %q", svc.createReq.Type, "host anti-affinity")
	}
	if svc.createReq.Region != "yow-1" {
		t.Errorf("create request region = %q, want %q", svc.createReq.Region, "yow-1")
	}
}

func TestAffinityGroupResource_createResolvesSlugFromList(t *testing.T) {
	// The create response has no slug; the resource must fall back to the
	// list, matching on name AND type so a same-named group of another type
	// is never adopted.
	svc := &fakeAffinityGroupService{
		created: &affinitygroup.AffinityGroup{ID: "ag-uuid-1", Name: "web-anti"},
		groups: []affinitygroup.AffinityGroup{
			{Slug: "web-anti-decoy", Name: "web-anti", Type: "host affinity"},
			{Slug: "web-anti-x1y2", Name: "web-anti", Type: "host anti-affinity"},
		},
	}
	resp := createAffinityGroup(t, svc, "web-anti", "host anti-affinity")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got affinityGroupStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "web-anti-x1y2" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "web-anti-x1y2")
	}
}

func TestAffinityGroupResource_createServiceError(t *testing.T) {
	svc := &fakeAffinityGroupService{err: errors.New("quota exceeded")}
	resp := createAffinityGroup(t, svc, "web-anti", "host anti-affinity")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func TestAffinityGroupResource_readFound(t *testing.T) {
	svc := &fakeAffinityGroupService{
		groups: []affinitygroup.AffinityGroup{
			{
				Slug:        "web-anti-x1y2",
				Name:        "web-anti-renamed",
				Type:        "host anti-affinity",
				Description: "spread web VMs",
				State:       "Active",
			},
		},
	}
	resp := readAffinityGroup(t, svc, "web-anti-x1y2")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got affinityGroupStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Name.ValueString() != "web-anti-renamed" {
		t.Errorf("Name = %q, want %q", got.Name.ValueString(), "web-anti-renamed")
	}
	if got.Description.ValueString() != "spread web VMs" {
		t.Errorf("Description = %q, want %q", got.Description.ValueString(), "spread web VMs")
	}
}

func TestAffinityGroupResource_readNotFound(t *testing.T) {
	svc := &fakeAffinityGroupService{}
	resp := readAffinityGroup(t, svc, "web-anti-x1y2")
	if resp.Diagnostics.HasError() {
		t.Fatalf("read-not-found should not produce diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestAffinityGroupResource_deleteHappyPath(t *testing.T) {
	svc := &fakeAffinityGroupService{}
	resp := deleteAffinityGroup(t, svc, "web-anti-x1y2")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "web-anti-x1y2" {
		t.Errorf("Delete called with %v, want [web-anti-x1y2]", svc.deleted)
	}
}

func TestAffinityGroupResource_delete404IsNoOp(t *testing.T) {
	svc := &fakeAffinityGroupService{err: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := deleteAffinityGroup(t, svc, "web-anti-x1y2")
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 on delete should be a no-op: %v", resp.Diagnostics)
	}
}
