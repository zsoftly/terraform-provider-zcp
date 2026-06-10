package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/project"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// projectStateModel mirrors projectDataSourceModel for state extraction in tests.
type projectStateModel struct {
	Slug        types.String `tfsdk:"slug"`
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

// fakeProjectLister is a test double that satisfies projectLister.
type fakeProjectLister struct {
	projects []project.Project
	err      error
}

func (f *fakeProjectLister) List(_ context.Context) ([]project.Project, error) {
	return f.projects, f.err
}

// readProjectDS calls Read on a fresh projectDataSource wired with lister.
func readProjectDS(t *testing.T, lister *fakeProjectLister, slug string) datasource.ReadResponse {
	t.Helper()

	// A nil *fakeProjectLister passed to NewProjectDataSourceWithLister would produce
	// a non-nil interface value (typed nil), bypassing the d.svc == nil guard.
	// Use the plain constructor so svc is a truly nil interface.
	var ds datasource.DataSource
	if lister != nil {
		ds = internalprovider.NewProjectDataSourceWithLister(lister)
	} else {
		ds = internalprovider.NewProjectDataSource()
	}

	schResp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, schResp)

	tfType := schResp.Schema.Type().TerraformType(context.Background())
	configVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"slug":        tftypes.NewValue(tftypes.String, slug),
		"id":          tftypes.NewValue(tftypes.String, nil),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
	})

	readReq := datasource.ReadRequest{
		Config: tfsdk.Config{
			Schema: schResp.Schema,
			Raw:    configVal,
		},
	}
	readResp := &datasource.ReadResponse{
		State: tfsdk.State{
			Schema: schResp.Schema,
			Raw:    tftypes.NewValue(tfType, nil),
		},
	}
	ds.Read(context.Background(), readReq, readResp)
	return *readResp
}

// AC: slug matches → no error, id/name/description written to state.
func TestProjectDataSource_found(t *testing.T) {
	lister := &fakeProjectLister{
		projects: []project.Project{
			{ID: "abc-123", Slug: "default", Name: "Default Project", Description: "Main"},
		},
	}
	resp := readProjectDS(t, lister, "default")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %s", resp.Diagnostics.Errors()[0].Detail())
	}

	var got projectStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "abc-123" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "abc-123")
	}
	if got.Name.ValueString() != "Default Project" {
		t.Errorf("Name = %q, want %q", got.Name.ValueString(), "Default Project")
	}
	if got.Description.ValueString() != "Main" {
		t.Errorf("Description = %q, want %q", got.Description.ValueString(), "Main")
	}
}

// AC: non-existent slug → diagnostic error, no panic.
func TestProjectDataSource_notFound(t *testing.T) {
	lister := &fakeProjectLister{projects: []project.Project{}}
	resp := readProjectDS(t, lister, "missing")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for missing slug, got none")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Project not found" {
		t.Errorf("unexpected summary: %q", got)
	}
}

// API error propagates as a diagnostic.
func TestProjectDataSource_listError(t *testing.T) {
	lister := &fakeProjectLister{err: errors.New("API unavailable")}
	resp := readProjectDS(t, lister, "default")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on list failure, got none")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Failed to list projects" {
		t.Errorf("unexpected summary: %q", got)
	}
}

// Read without a prior Configure returns a clear diagnostic, not a nil-pointer panic.
func TestProjectDataSource_readWithoutConfigure(t *testing.T) {
	resp := readProjectDS(t, nil, "default")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when svc is nil, got none")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Provider not configured" {
		t.Errorf("unexpected summary: %q", got)
	}
}

// Configure with nil ProviderData is a no-op (framework calls it during validation).
func TestProjectDataSource_configureNilProviderData(t *testing.T) {
	ds := internalprovider.NewProjectDataSource()
	configurable, ok := ds.(datasource.DataSourceWithConfigure)
	if !ok {
		t.Fatal("projectDataSource does not implement DataSourceWithConfigure")
	}
	req := datasource.ConfigureRequest{ProviderData: nil}
	var resp datasource.ConfigureResponse
	configurable.Configure(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure with nil ProviderData should be a no-op: %v", resp.Diagnostics)
	}
}
