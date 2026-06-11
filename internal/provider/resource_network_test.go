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
	"github.com/zsoftly/zcp-cli/pkg/api/network"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeNetworkService satisfies networkServiceIface.
type fakeNetworkService struct {
	networks []network.Network
	created  *network.Network
	updated  *network.Network
	err      error
	deleted  []string
}

func (f *fakeNetworkService) List(_ context.Context) ([]network.Network, error) {
	return f.networks, f.err
}
func (f *fakeNetworkService) Create(_ context.Context, _ network.CreateRequest) (*network.Network, error) {
	return f.created, f.err
}
func (f *fakeNetworkService) Update(_ context.Context, _ string, _ network.UpdateRequest) (*network.Network, error) {
	return f.updated, f.err
}
func (f *fakeNetworkService) Delete(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	return f.err
}

// networkStateModel mirrors networkResourceModel for state extraction in tests.
type networkStateModel struct {
	ID            types.String   `tfsdk:"id"`
	Name          types.String   `tfsdk:"name"`
	CloudProvider types.String   `tfsdk:"cloud_provider"`
	Region        types.String   `tfsdk:"region"`
	Project       types.String   `tfsdk:"project"`
	Description   types.String   `tfsdk:"description"`
	CategorySlug  types.String   `tfsdk:"category_slug"`
	Gateway       types.String   `tfsdk:"gateway"`
	CIDR          types.String   `tfsdk:"cidr"`
	Netmask       types.String   `tfsdk:"netmask"`
	Timeouts      timeouts.Value `tfsdk:"timeouts"`
}

func networkSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewNetworkResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func networkTFType(t *testing.T) tftypes.Type {
	t.Helper()
	return networkSchema(t).Schema.Type().TerraformType(context.Background())
}

func createNetwork(t *testing.T, svc *fakeNetworkService, name, region, provider string) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewNetworkResourceWithService(svc)
	schResp := networkSchema(t)
	tfType := networkTFType(t)
	planVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"name":           tftypes.NewValue(tftypes.String, name),
		"cloud_provider": tftypes.NewValue(tftypes.String, provider),
		"region":         tftypes.NewValue(tftypes.String, region),
		"project":        tftypes.NewValue(tftypes.String, nil),
		"description":    tftypes.NewValue(tftypes.String, nil),
		"category_slug":  tftypes.NewValue(tftypes.String, nil),
		"gateway":        tftypes.NewValue(tftypes.String, nil),
		"cidr":           tftypes.NewValue(tftypes.String, nil),
		"netmask":        tftypes.NewValue(tftypes.String, nil),
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

func readNetwork(t *testing.T, svc *fakeNetworkService, slug string) resource.ReadResponse {
	t.Helper()
	var r resource.Resource
	if svc != nil {
		r = internalprovider.NewNetworkResourceWithService(svc)
	} else {
		r = internalprovider.NewNetworkResource()
	}
	schResp := networkSchema(t)
	tfType := networkTFType(t)
	stateVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, slug),
		"name":           tftypes.NewValue(tftypes.String, "testnet"),
		"cloud_provider": tftypes.NewValue(tftypes.String, "cloudstack"),
		"region":         tftypes.NewValue(tftypes.String, "yow"),
		"project":        tftypes.NewValue(tftypes.String, nil),
		"description":    tftypes.NewValue(tftypes.String, nil),
		"category_slug":  tftypes.NewValue(tftypes.String, "isolated"),
		"gateway":        tftypes.NewValue(tftypes.String, "10.0.0.1"),
		"cidr":           tftypes.NewValue(tftypes.String, "10.0.0.0/24"),
		"netmask":        tftypes.NewValue(tftypes.String, "255.255.255.0"),
		"timeouts":       timeoutsNull(t, schResp),
	})
	readReq := resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), readReq, readResp)
	return *readResp
}

func deleteNetwork(t *testing.T, svc *fakeNetworkService, slug string) resource.DeleteResponse {
	t.Helper()
	r := internalprovider.NewNetworkResourceWithService(svc)
	schResp := networkSchema(t)
	tfType := networkTFType(t)
	stateVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, slug),
		"name":           tftypes.NewValue(tftypes.String, "testnet"),
		"cloud_provider": tftypes.NewValue(tftypes.String, "cloudstack"),
		"region":         tftypes.NewValue(tftypes.String, "yow"),
		"project":        tftypes.NewValue(tftypes.String, nil),
		"description":    tftypes.NewValue(tftypes.String, nil),
		"category_slug":  tftypes.NewValue(tftypes.String, nil),
		"gateway":        tftypes.NewValue(tftypes.String, "10.0.0.1"),
		"cidr":           tftypes.NewValue(tftypes.String, "10.0.0.0/24"),
		"netmask":        tftypes.NewValue(tftypes.String, "255.255.255.0"),
		"timeouts":       timeoutsNull(t, schResp),
	})
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

func TestNetworkResource_createHappyPath(t *testing.T) {
	svc := &fakeNetworkService{
		created: &network.Network{
			Slug:    "testnet-abc123",
			Name:    "testnet",
			Gateway: "10.0.0.1",
			CIDR:    "10.0.0.0/24",
			Netmask: "255.255.255.0",
		},
	}
	resp := createNetwork(t, svc, "testnet", "yow", "cloudstack")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got networkStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "testnet-abc123" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "testnet-abc123")
	}
	if got.Gateway.ValueString() != "10.0.0.1" {
		t.Errorf("Gateway = %q, want %q", got.Gateway.ValueString(), "10.0.0.1")
	}
	if got.CIDR.ValueString() != "10.0.0.0/24" {
		t.Errorf("CIDR = %q, want %q", got.CIDR.ValueString(), "10.0.0.0/24")
	}
}

func TestNetworkResource_createServiceError(t *testing.T) {
	svc := &fakeNetworkService{err: errors.New("quota exceeded")}
	resp := createNetwork(t, svc, "testnet", "yow", "cloudstack")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func TestNetworkResource_readFound(t *testing.T) {
	svc := &fakeNetworkService{
		networks: []network.Network{
			{Slug: "testnet-abc123", Name: "updated-name", Description: "desc", Gateway: "10.0.0.1", CIDR: "10.0.0.0/24", Netmask: "255.255.255.0"},
		},
	}
	resp := readNetwork(t, svc, "testnet-abc123")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got networkStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Name.ValueString() != "updated-name" {
		t.Errorf("Name = %q, want %q", got.Name.ValueString(), "updated-name")
	}
	// category_slug must be preserved from state, not overwritten from API response.
	if got.CategorySlug.ValueString() != "isolated" {
		t.Errorf("CategorySlug = %q, want preserved value %q", got.CategorySlug.ValueString(), "isolated")
	}
}

func TestNetworkResource_readNotFound(t *testing.T) {
	svc := &fakeNetworkService{networks: []network.Network{}}
	resp := readNetwork(t, svc, "missing-slug")
	if resp.Diagnostics.HasError() {
		t.Fatalf("read-not-found should not produce diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestNetworkResource_readWithoutConfigure(t *testing.T) {
	resp := readNetwork(t, nil, "any-slug")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when svc is nil, got none")
	}
}

func TestNetworkResource_deleteHappyPath(t *testing.T) {
	svc := &fakeNetworkService{}
	resp := deleteNetwork(t, svc, "testnet-abc123")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "testnet-abc123" {
		t.Errorf("Delete called with %v, want [testnet-abc123]", svc.deleted)
	}
}

func TestNetworkResource_delete404IsNoOp(t *testing.T) {
	svc := &fakeNetworkService{err: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := deleteNetwork(t, svc, "gone-slug")
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 on delete should be a no-op: %v", resp.Diagnostics)
	}
}

func TestNetworkResource_updateNameDescription(t *testing.T) {
	svc := &fakeNetworkService{
		networks: []network.Network{
			{Slug: "testnet-abc123", Name: "new-name", Description: "new-desc"},
		},
		updated: &network.Network{Slug: "testnet-abc123", Name: "new-name", Description: "new-desc"},
	}
	r := internalprovider.NewNetworkResourceWithService(svc)
	schResp := networkSchema(t)
	tfType := networkTFType(t)

	existingState := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "testnet-abc123"),
		"name":           tftypes.NewValue(tftypes.String, "old-name"),
		"cloud_provider": tftypes.NewValue(tftypes.String, "cloudstack"),
		"region":         tftypes.NewValue(tftypes.String, "yow"),
		"project":        tftypes.NewValue(tftypes.String, nil),
		"description":    tftypes.NewValue(tftypes.String, "old-desc"),
		"category_slug":  tftypes.NewValue(tftypes.String, "isolated"),
		"gateway":        tftypes.NewValue(tftypes.String, "10.0.0.1"),
		"cidr":           tftypes.NewValue(tftypes.String, "10.0.0.0/24"),
		"netmask":        tftypes.NewValue(tftypes.String, "255.255.255.0"),
		"timeouts":       timeoutsNull(t, schResp),
	})
	newPlan := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "testnet-abc123"),
		"name":           tftypes.NewValue(tftypes.String, "new-name"),
		"cloud_provider": tftypes.NewValue(tftypes.String, "cloudstack"),
		"region":         tftypes.NewValue(tftypes.String, "yow"),
		"project":        tftypes.NewValue(tftypes.String, nil),
		"description":    tftypes.NewValue(tftypes.String, "new-desc"),
		"category_slug":  tftypes.NewValue(tftypes.String, "isolated"),
		"gateway":        tftypes.NewValue(tftypes.String, nil),
		"cidr":           tftypes.NewValue(tftypes.String, nil),
		"netmask":        tftypes.NewValue(tftypes.String, nil),
		"timeouts":       timeoutsNull(t, schResp),
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
	var got networkStateModel
	if diags := updateResp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Name.ValueString() != "new-name" {
		t.Errorf("Name = %q, want %q", got.Name.ValueString(), "new-name")
	}
	if got.CategorySlug.ValueString() != "isolated" {
		t.Errorf("CategorySlug = %q, want preserved value %q", got.CategorySlug.ValueString(), "isolated")
	}
	if got.Gateway.ValueString() != "10.0.0.1" {
		t.Errorf("Gateway = %q, want preserved value %q", got.Gateway.ValueString(), "10.0.0.1")
	}
}

// When the planned description is empty (user tries to clear it), the API cannot honour
// the clear (omitempty), so the Update must preserve the prior state description to keep
// Terraform state consistent with the remote resource.
func TestNetworkResource_updateClearsDescriptionWhenEmpty(t *testing.T) {
	svc := &fakeNetworkService{
		updated: &network.Network{Slug: "testnet-abc123", Name: "same-name"},
	}
	r := internalprovider.NewNetworkResourceWithService(svc)
	schResp := networkSchema(t)
	tfType := networkTFType(t)

	existingState := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "testnet-abc123"),
		"name":           tftypes.NewValue(tftypes.String, "same-name"),
		"cloud_provider": tftypes.NewValue(tftypes.String, "cloudstack"),
		"region":         tftypes.NewValue(tftypes.String, "yow"),
		"project":        tftypes.NewValue(tftypes.String, nil),
		"description":    tftypes.NewValue(tftypes.String, "old-desc"),
		"category_slug":  tftypes.NewValue(tftypes.String, nil),
		"gateway":        tftypes.NewValue(tftypes.String, "10.0.0.1"),
		"cidr":           tftypes.NewValue(tftypes.String, "10.0.0.0/24"),
		"netmask":        tftypes.NewValue(tftypes.String, "255.255.255.0"),
		"timeouts":       timeoutsNull(t, schResp),
	})
	// Plan has empty description (user set description = "").
	emptyDescPlan := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "testnet-abc123"),
		"name":           tftypes.NewValue(tftypes.String, "same-name"),
		"cloud_provider": tftypes.NewValue(tftypes.String, "cloudstack"),
		"region":         tftypes.NewValue(tftypes.String, "yow"),
		"project":        tftypes.NewValue(tftypes.String, nil),
		"description":    tftypes.NewValue(tftypes.String, ""),
		"category_slug":  tftypes.NewValue(tftypes.String, nil),
		"gateway":        tftypes.NewValue(tftypes.String, nil),
		"cidr":           tftypes.NewValue(tftypes.String, nil),
		"netmask":        tftypes.NewValue(tftypes.String, nil),
		"timeouts":       timeoutsNull(t, schResp),
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
	var got networkStateModel
	if diags := updateResp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	// Empty description must flow through to state — not silently overridden with the old value.
	if got.Description.ValueString() != "" {
		t.Errorf("Description = %q, want %q (empty description should be honoured)", got.Description.ValueString(), "")
	}
}
