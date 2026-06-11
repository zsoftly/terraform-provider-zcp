package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/region"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

type fakeRegionLister struct {
	regions []region.Region
	err     error
}

func (f *fakeRegionLister) List(_ context.Context) ([]region.Region, error) {
	return f.regions, f.err
}

type regionStateModel struct {
	Slug          types.String `tfsdk:"slug"`
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Country       types.String `tfsdk:"country"`
	CountryCode   types.String `tfsdk:"country_code"`
	CloudProvider types.String `tfsdk:"cloud_provider"`
}

func readRegionDS(t *testing.T, lister *fakeRegionLister, slug string) datasource.ReadResponse {
	t.Helper()

	var ds datasource.DataSource
	if lister != nil {
		ds = internalprovider.NewRegionDataSourceWithLister(lister)
	} else {
		ds = internalprovider.NewRegionDataSource()
	}

	schResp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, schResp)

	tfType := schResp.Schema.Type().TerraformType(context.Background())
	configVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"slug":           tftypes.NewValue(tftypes.String, slug),
		"id":             tftypes.NewValue(tftypes.String, nil),
		"name":           tftypes.NewValue(tftypes.String, nil),
		"country":        tftypes.NewValue(tftypes.String, nil),
		"country_code":   tftypes.NewValue(tftypes.String, nil),
		"cloud_provider": tftypes.NewValue(tftypes.String, nil),
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

// AC: slug matches → state populated with all attributes including cloud_provider.
// Values match the real ZCP API response (verified 2026-06-10):
//
//	yow-1 → name "YOW-1", cloud_provider "nimbo", country "Canada"/"CA"
//	yul-1 → name "YUL-1", cloud_provider "nimbo", country "Canada"/"CA"
func TestRegionDataSource_found(t *testing.T) {
	lister := &fakeRegionLister{
		regions: []region.Region{
			{
				ID:            "a19c99c6-7138-42c1-849d-4d888447c85d",
				Slug:          "yow-1",
				Name:          "YOW-1",
				Country:       "Canada",
				CountryCode:   "CA",
				CloudProvider: &region.CloudProvider{Slug: "nimbo"},
			},
		},
	}
	resp := readRegionDS(t, lister, "yow-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %s", resp.Diagnostics.Errors()[0].Detail())
	}

	var got regionStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	checks := map[string][2]string{
		"ID":            {got.ID.ValueString(), "a19c99c6-7138-42c1-849d-4d888447c85d"},
		"Name":          {got.Name.ValueString(), "YOW-1"},
		"Country":       {got.Country.ValueString(), "Canada"},
		"CountryCode":   {got.CountryCode.ValueString(), "CA"},
		"CloudProvider": {got.CloudProvider.ValueString(), "nimbo"},
	}
	for field, pair := range checks {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, want %q", field, pair[0], pair[1])
		}
	}
}

// When the API returns a region without a cloud_provider (nil), cloud_provider is "".
// The object-storage regions (os-yow, os-yul) use the "ceph" provider; the DNS
// region uses "dns". Only compute regions (yow-1, yul-1) use "nimbo".
func TestRegionDataSource_nilCloudProvider(t *testing.T) {
	lister := &fakeRegionLister{
		regions: []region.Region{
			{
				ID:            "a1c28c40-3961-467f-8cad-84b480208a42",
				Slug:          "yul-1",
				Name:          "YUL-1",
				Country:       "Canada",
				CountryCode:   "CA",
				CloudProvider: nil,
			},
		},
	}
	resp := readRegionDS(t, lister, "yul-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got regionStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.CloudProvider.ValueString() != "" {
		t.Errorf("CloudProvider = %q, want empty string for nil provider", got.CloudProvider.ValueString())
	}
}

// AC: unknown slug returns a clear diagnostic.
func TestRegionDataSource_notFound(t *testing.T) {
	lister := &fakeRegionLister{regions: []region.Region{
		{ID: "a19c99c6-7138-42c1-849d-4d888447c85d", Slug: "yow-1", Name: "YOW-1", Country: "Canada", CountryCode: "CA", CloudProvider: &region.CloudProvider{Slug: "nimbo"}},
	}}
	resp := readRegionDS(t, lister, "nowhere")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for missing slug, got none")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Region not found" {
		t.Errorf("unexpected summary: %q", got)
	}
}

func TestRegionDataSource_listError(t *testing.T) {
	lister := &fakeRegionLister{err: errors.New("API unavailable")}
	resp := readRegionDS(t, lister, "yow-1")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on list failure, got none")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Failed to list regions" {
		t.Errorf("unexpected summary: %q", got)
	}
}

func TestRegionDataSource_readWithoutConfigure(t *testing.T) {
	resp := readRegionDS(t, nil, "yow-1")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when svc is nil, got none")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Provider not configured" {
		t.Errorf("unexpected summary: %q", got)
	}
}

func TestRegionDataSource_configureNilProviderData(t *testing.T) {
	ds := internalprovider.NewRegionDataSource()
	configurable, ok := ds.(datasource.DataSourceWithConfigure)
	if !ok {
		t.Fatal("regionDataSource does not implement DataSourceWithConfigure")
	}
	req := datasource.ConfigureRequest{ProviderData: nil}
	var resp datasource.ConfigureResponse
	configurable.Configure(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure with nil ProviderData should be a no-op: %v", resp.Diagnostics)
	}
}
