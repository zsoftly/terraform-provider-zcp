package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/template"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

type fakeTemplateLister struct {
	templates  []template.Template
	err        error
	calledWith string // captures the regionSlug passed to List
}

func (f *fakeTemplateLister) List(_ context.Context, regionSlug string) ([]template.Template, error) {
	f.calledWith = regionSlug
	return f.templates, f.err
}

type templateStateModel struct {
	Slug       types.String `tfsdk:"slug"`
	RegionSlug types.String `tfsdk:"region_slug"`
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	RegionID   types.String `tfsdk:"region_id"`
	Type       types.String `tfsdk:"type"`
}

func readTemplateDS(t *testing.T, lister *fakeTemplateLister, slug, regionSlug string) datasource.ReadResponse {
	t.Helper()

	var ds datasource.DataSource
	if lister != nil {
		ds = internalprovider.NewTemplateDataSourceWithLister(lister)
	} else {
		ds = internalprovider.NewTemplateDataSource()
	}

	schResp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, schResp)

	tfType := schResp.Schema.Type().TerraformType(context.Background())

	var regionSlugVal tftypes.Value
	if regionSlug != "" {
		regionSlugVal = tftypes.NewValue(tftypes.String, regionSlug)
	} else {
		regionSlugVal = tftypes.NewValue(tftypes.String, nil)
	}

	configVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"slug":        tftypes.NewValue(tftypes.String, slug),
		"region_slug": regionSlugVal,
		"id":          tftypes.NewValue(tftypes.String, nil),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"region_id":   tftypes.NewValue(tftypes.String, nil),
		"type":        tftypes.NewValue(tftypes.String, nil),
	})

	readReq := datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schResp.Schema, Raw: configVal},
	}
	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)},
	}
	ds.Read(context.Background(), readReq, readResp)
	return *readResp
}

func TestTemplateDataSource_found(t *testing.T) {
	lister := &fakeTemplateLister{
		templates: []template.Template{
			{ID: "t-001", Slug: "ubuntu-2204-lts", Name: "Ubuntu-22.04-LTS", RegionID: "r-001", Type: "Template"},
		},
	}
	resp := readTemplateDS(t, lister, "ubuntu-2204-lts", "yow-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %s", resp.Diagnostics.Errors()[0].Detail())
	}

	var got templateStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	checks := map[string][2]string{
		"ID":       {got.ID.ValueString(), "t-001"},
		"Name":     {got.Name.ValueString(), "Ubuntu-22.04-LTS"},
		"RegionID": {got.RegionID.ValueString(), "r-001"},
		"Type":     {got.Type.ValueString(), "Template"},
	}
	for field, pair := range checks {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, want %q", field, pair[0], pair[1])
		}
	}
	if lister.calledWith != "yow-1" {
		t.Errorf("List called with regionSlug=%q, want %q", lister.calledWith, "yow-1")
	}
}

func TestTemplateDataSource_noRegionSlug(t *testing.T) {
	lister := &fakeTemplateLister{
		templates: []template.Template{
			{ID: "t-001", Slug: "ubuntu-2204-lts", Name: "Ubuntu-22.04-LTS", RegionID: "r-001", Type: "Template"},
		},
	}
	resp := readTemplateDS(t, lister, "ubuntu-2204-lts", "")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %s", resp.Diagnostics.Errors()[0].Detail())
	}
	if lister.calledWith != "" {
		t.Errorf("expected empty regionSlug, got %q", lister.calledWith)
	}
}

func TestTemplateDataSource_notFound(t *testing.T) {
	lister := &fakeTemplateLister{templates: []template.Template{}}
	resp := readTemplateDS(t, lister, "does-not-exist", "yow-1")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for missing slug, got none")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Template not found" {
		t.Errorf("unexpected summary: %q", got)
	}
}

func TestTemplateDataSource_listError(t *testing.T) {
	lister := &fakeTemplateLister{err: errors.New("API unavailable")}
	resp := readTemplateDS(t, lister, "ubuntu-2204-lts", "")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on list failure, got none")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Failed to list templates" {
		t.Errorf("unexpected summary: %q", got)
	}
}

func TestTemplateDataSource_readWithoutConfigure(t *testing.T) {
	resp := readTemplateDS(t, nil, "ubuntu-2204-lts", "")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when svc is nil, got none")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Provider not configured" {
		t.Errorf("unexpected summary: %q", got)
	}
}

func TestTemplateDataSource_configureNilProviderData(t *testing.T) {
	ds := internalprovider.NewTemplateDataSource()
	configurable, ok := ds.(datasource.DataSourceWithConfigure)
	if !ok {
		t.Fatal("templateDataSource does not implement DataSourceWithConfigure")
	}
	req := datasource.ConfigureRequest{ProviderData: nil}
	var resp datasource.ConfigureResponse
	configurable.Configure(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure with nil ProviderData should be a no-op: %v", resp.Diagnostics)
	}
}
