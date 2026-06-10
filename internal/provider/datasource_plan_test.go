package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/plan"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

type fakePlanLister struct {
	plans      []plan.Plan
	err        error
	calledWith plan.ServiceType
}

func (f *fakePlanLister) List(_ context.Context, svc plan.ServiceType) ([]plan.Plan, error) {
	f.calledWith = svc
	return f.plans, f.err
}

type planStateModel struct {
	Slug         types.String  `tfsdk:"slug"`
	Service      types.String  `tfsdk:"service"`
	ID           types.String  `tfsdk:"id"`
	Name         types.String  `tfsdk:"name"`
	CPU          types.Int64   `tfsdk:"cpu"`
	MemoryMB     types.Int64   `tfsdk:"memory_mb"`
	StorageGB    types.Int64   `tfsdk:"storage_gb"`
	HourlyPrice  types.Float64 `tfsdk:"hourly_price"`
	MonthlyPrice types.Float64 `tfsdk:"monthly_price"`
}

func testPlan(slug string) plan.Plan {
	return plan.Plan{
		ID:   "p-001",
		Name: "ci1.xs",
		Slug: slug,
		Attribute: plan.Attribute{
			CPU:     json.Number("1"),
			Memory:  json.Number("1024"),
			Storage: json.Number("40"),
		},
		HourlyPrice:  0.009,
		MonthlyPrice: 6.5,
	}
}

func readPlanDS(t *testing.T, lister *fakePlanLister, slug, service string) datasource.ReadResponse {
	t.Helper()

	var ds datasource.DataSource
	if lister != nil {
		ds = internalprovider.NewPlanDataSourceWithLister(lister)
	} else {
		ds = internalprovider.NewPlanDataSource()
	}

	schResp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, schResp)

	tfType := schResp.Schema.Type().TerraformType(context.Background())

	var svcVal tftypes.Value
	if service != "" {
		svcVal = tftypes.NewValue(tftypes.String, service)
	} else {
		svcVal = tftypes.NewValue(tftypes.String, nil)
	}

	configVal := tftypes.NewValue(tfType, map[string]tftypes.Value{
		"slug":          tftypes.NewValue(tftypes.String, slug),
		"service":       svcVal,
		"id":            tftypes.NewValue(tftypes.String, nil),
		"name":          tftypes.NewValue(tftypes.String, nil),
		"cpu":           tftypes.NewValue(tftypes.Number, nil),
		"memory_mb":     tftypes.NewValue(tftypes.Number, nil),
		"storage_gb":    tftypes.NewValue(tftypes.Number, nil),
		"hourly_price":  tftypes.NewValue(tftypes.Number, nil),
		"monthly_price": tftypes.NewValue(tftypes.Number, nil),
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

func TestPlanDataSource_found(t *testing.T) {
	lister := &fakePlanLister{plans: []plan.Plan{testPlan("ci1xs")}}
	resp := readPlanDS(t, lister, "ci1xs", "")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %s", resp.Diagnostics.Errors()[0].Detail())
	}

	var got planStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "p-001" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "p-001")
	}
	if got.CPU.ValueInt64() != 1 {
		t.Errorf("CPU = %d, want 1", got.CPU.ValueInt64())
	}
	if got.MemoryMB.ValueInt64() != 1024 {
		t.Errorf("MemoryMB = %d, want 1024", got.MemoryMB.ValueInt64())
	}
	if got.StorageGB.ValueInt64() != 40 {
		t.Errorf("StorageGB = %d, want 40", got.StorageGB.ValueInt64())
	}
	if lister.calledWith != plan.ServiceVM {
		t.Errorf("List called with %q, want %q", lister.calledWith, plan.ServiceVM)
	}
}

func TestPlanDataSource_customService(t *testing.T) {
	lister := &fakePlanLister{plans: []plan.Plan{testPlan("k8s-small")}}
	resp := readPlanDS(t, lister, "k8s-small", "Kubernetes")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %s", resp.Diagnostics.Errors()[0].Detail())
	}
	if lister.calledWith != plan.ServiceKubernetes {
		t.Errorf("List called with %q, want %q", lister.calledWith, plan.ServiceKubernetes)
	}
}

func TestPlanDataSource_notFound(t *testing.T) {
	lister := &fakePlanLister{plans: []plan.Plan{}}
	resp := readPlanDS(t, lister, "ghost-plan", "")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for missing slug, got none")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Plan not found" {
		t.Errorf("unexpected summary: %q", got)
	}
}

func TestPlanDataSource_listError(t *testing.T) {
	lister := &fakePlanLister{err: errors.New("API unavailable")}
	resp := readPlanDS(t, lister, "ci1xs", "")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on list failure, got none")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Failed to list plans" {
		t.Errorf("unexpected summary: %q", got)
	}
}

func TestPlanDataSource_readWithoutConfigure(t *testing.T) {
	resp := readPlanDS(t, nil, "ci1xs", "")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when svc is nil, got none")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Provider not configured" {
		t.Errorf("unexpected summary: %q", got)
	}
}

func TestPlanDataSource_configureNilProviderData(t *testing.T) {
	ds := internalprovider.NewPlanDataSource()
	configurable, ok := ds.(datasource.DataSourceWithConfigure)
	if !ok {
		t.Fatal("planDataSource does not implement DataSourceWithConfigure")
	}
	req := datasource.ConfigureRequest{ProviderData: nil}
	var resp datasource.ConfigureResponse
	configurable.Configure(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure with nil ProviderData should be a no-op: %v", resp.Diagnostics)
	}
}
