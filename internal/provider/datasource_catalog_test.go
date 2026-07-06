package provider_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/billingcycle"
	"github.com/zsoftly/zcp-cli/pkg/api/kubernetes"
	"github.com/zsoftly/zcp-cli/pkg/api/network"
	"github.com/zsoftly/zcp-cli/pkg/api/sshkey"
	"github.com/zsoftly/zcp-cli/pkg/api/storagecategory"
	"github.com/zsoftly/zcp-cli/pkg/api/vpc"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// readDS drives a data source Read with the given config attribute values.
// Unset schema attributes default to null.
func readDS(t *testing.T, ds datasource.DataSource, config map[string]tftypes.Value) datasource.ReadResponse {
	t.Helper()
	schResp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, schResp)
	tfType := schResp.Schema.Type().TerraformType(context.Background())

	obj, ok := tfType.(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is %T, want tftypes.Object", tfType)
	}
	vals := map[string]tftypes.Value{}
	for name, attrType := range obj.AttributeTypes {
		if v, set := config[name]; set {
			vals[name] = v
		} else {
			vals[name] = tftypes.NewValue(attrType, nil)
		}
	}

	readReq := datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, vals)},
	}
	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)},
	}
	ds.Read(context.Background(), readReq, readResp)
	return *readResp
}

// --- zcp_storage_category ---

type fakeStorageCategoryLister struct {
	categories []storagecategory.StorageCategory
	gotRegion  string
}

func (f *fakeStorageCategoryLister) List(_ context.Context, regionSlug string) ([]storagecategory.StorageCategory, error) {
	f.gotRegion = regionSlug
	return f.categories, nil
}

func TestStorageCategoryDataSource_found(t *testing.T) {
	lister := &fakeStorageCategoryLister{
		categories: []storagecategory.StorageCategory{
			{ID: "sc-1", Slug: "pro-nvme", Name: "Pro NVMe", Status: true},
		},
	}
	resp := readDS(t, internalprovider.NewStorageCategoryDataSourceWithLister(lister), map[string]tftypes.Value{
		"slug":   strVal("pro-nvme"),
		"region": strVal("yul-1"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got struct {
		Slug   types.String `tfsdk:"slug"`
		Region types.String `tfsdk:"region"`
		ID     types.String `tfsdk:"id"`
		Name   types.String `tfsdk:"name"`
		Status types.Bool   `tfsdk:"status"`
	}
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "sc-1" || got.Name.ValueString() != "Pro NVMe" {
		t.Errorf("got id=%q name=%q, want sc-1 / Pro NVMe", got.ID.ValueString(), got.Name.ValueString())
	}
	if lister.gotRegion != "yul-1" {
		t.Errorf("List called with region %q, want yul-1", lister.gotRegion)
	}
}

func TestStorageCategoryDataSource_notFound(t *testing.T) {
	lister := &fakeStorageCategoryLister{}
	resp := readDS(t, internalprovider.NewStorageCategoryDataSourceWithLister(lister), map[string]tftypes.Value{
		"slug": strVal("nvme"),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected not-found error, got none")
	}
}

// --- zcp_billing_cycle ---

type fakeBillingCycleLister struct {
	cycles []billingcycle.BillingCycle
}

func (f *fakeBillingCycleLister) List(_ context.Context) ([]billingcycle.BillingCycle, error) {
	return f.cycles, nil
}

func TestBillingCycleDataSource_found(t *testing.T) {
	lister := &fakeBillingCycleLister{
		cycles: []billingcycle.BillingCycle{
			{ID: "bc-1", Slug: "hourly", Name: "Hourly", Duration: 1, Unit: "hour", IsEnabled: true},
		},
	}
	resp := readDS(t, internalprovider.NewBillingCycleDataSourceWithLister(lister), map[string]tftypes.Value{
		"slug": strVal("hourly"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got struct {
		Slug        types.String `tfsdk:"slug"`
		ID          types.String `tfsdk:"id"`
		Name        types.String `tfsdk:"name"`
		Description types.String `tfsdk:"description"`
		Duration    types.Int64  `tfsdk:"duration"`
		Unit        types.String `tfsdk:"unit"`
		IsEnabled   types.Bool   `tfsdk:"is_enabled"`
	}
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "bc-1" || got.Duration.ValueInt64() != 1 {
		t.Errorf("got id=%q duration=%d, want bc-1 / 1", got.ID.ValueString(), got.Duration.ValueInt64())
	}
}

func TestBillingCycleDataSource_notFound(t *testing.T) {
	resp := readDS(t, internalprovider.NewBillingCycleDataSourceWithLister(&fakeBillingCycleLister{}), map[string]tftypes.Value{
		"slug": strVal("yearly"),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected not-found error, got none")
	}
}

// --- zcp_network (data source) ---

type fakeNetworkLister struct {
	networks []network.Network
}

func (f *fakeNetworkLister) List(_ context.Context, _, _ string) ([]network.Network, error) {
	return f.networks, nil
}

func TestNetworkDataSource_found(t *testing.T) {
	lister := &fakeNetworkLister{
		networks: []network.Network{
			{Slug: "prod-net", Name: "prod", NetworkType: "Isolated", CIDR: "10.1.1.0/24", Gateway: "10.1.1.1", IsDefault: true},
		},
	}
	resp := readDS(t, internalprovider.NewNetworkDataSourceWithLister(lister), map[string]tftypes.Value{
		"slug": strVal("prod-net"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got struct {
		Slug        types.String `tfsdk:"slug"`
		Region      types.String `tfsdk:"region"`
		Project     types.String `tfsdk:"project"`
		ID          types.String `tfsdk:"id"`
		Name        types.String `tfsdk:"name"`
		Type        types.String `tfsdk:"type"`
		Gateway     types.String `tfsdk:"gateway"`
		CIDR        types.String `tfsdk:"cidr"`
		Netmask     types.String `tfsdk:"netmask"`
		Category    types.String `tfsdk:"category"`
		VPC         types.String `tfsdk:"vpc"`
		IsDefault   types.Bool   `tfsdk:"is_default"`
		ZoneName    types.String `tfsdk:"zone_name"`
		Description types.String `tfsdk:"description"`
	}
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "prod-net" || got.CIDR.ValueString() != "10.1.1.0/24" {
		t.Errorf("got id=%q cidr=%q, want prod-net / 10.1.1.0/24", got.ID.ValueString(), got.CIDR.ValueString())
	}
}

func TestNetworkDataSource_notFound(t *testing.T) {
	resp := readDS(t, internalprovider.NewNetworkDataSourceWithLister(&fakeNetworkLister{}), map[string]tftypes.Value{
		"slug": strVal("missing"),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected not-found error, got none")
	}
}

// --- zcp_vpc (data source) ---

type fakeVPCLister struct {
	vpcs []vpc.VPC
}

func (f *fakeVPCLister) List(_ context.Context, _, _, _ string) ([]vpc.VPC, error) {
	return f.vpcs, nil
}

func TestVPCDataSource_found(t *testing.T) {
	lister := &fakeVPCLister{
		vpcs: []vpc.VPC{
			{Slug: "main-vpc", Name: "main", Status: "Enabled", CIDR: "10.1.0.0/16"},
		},
	}
	resp := readDS(t, internalprovider.NewVPCDataSourceWithLister(lister), map[string]tftypes.Value{
		"slug": strVal("main-vpc"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got struct {
		Slug        types.String `tfsdk:"slug"`
		Region      types.String `tfsdk:"region"`
		Project     types.String `tfsdk:"project"`
		ID          types.String `tfsdk:"id"`
		Name        types.String `tfsdk:"name"`
		Description types.String `tfsdk:"description"`
		Status      types.String `tfsdk:"status"`
		CIDR        types.String `tfsdk:"cidr"`
		ZoneName    types.String `tfsdk:"zone_name"`
		DomainName  types.String `tfsdk:"domain_name"`
	}
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "main-vpc" || got.Status.ValueString() != "Enabled" {
		t.Errorf("got id=%q status=%q, want main-vpc / Enabled", got.ID.ValueString(), got.Status.ValueString())
	}
}

func TestVPCDataSource_notFound(t *testing.T) {
	resp := readDS(t, internalprovider.NewVPCDataSourceWithLister(&fakeVPCLister{}), map[string]tftypes.Value{
		"slug": strVal("missing"),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected not-found error, got none")
	}
}

// --- zcp_ssh_key (data source) ---

type fakeSSHKeyLister struct {
	keys []sshkey.SSHKey
}

func (f *fakeSSHKeyLister) List(_ context.Context) ([]sshkey.SSHKey, error) {
	return f.keys, nil
}

func TestSSHKeyDataSource_foundByName(t *testing.T) {
	lister := &fakeSSHKeyLister{
		keys: []sshkey.SSHKey{
			{Slug: "deploy-x1", Name: "deploy", PublicKey: "ssh-ed25519 AAAA", CreatedAt: "2026-01-01"},
		},
	}
	resp := readDS(t, internalprovider.NewSSHKeyDataSourceWithLister(lister), map[string]tftypes.Value{
		"name": strVal("deploy"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got struct {
		Slug      types.String `tfsdk:"slug"`
		Name      types.String `tfsdk:"name"`
		ID        types.String `tfsdk:"id"`
		PublicKey types.String `tfsdk:"public_key"`
		CreatedAt types.String `tfsdk:"created_at"`
	}
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "deploy-x1" || got.PublicKey.ValueString() != "ssh-ed25519 AAAA" {
		t.Errorf("got id=%q key=%q, want deploy-x1 / ssh-ed25519 AAAA", got.ID.ValueString(), got.PublicKey.ValueString())
	}
}

func TestSSHKeyDataSource_requiresExactlyOneLookup(t *testing.T) {
	lister := &fakeSSHKeyLister{}
	// Neither slug nor name.
	resp := readDS(t, internalprovider.NewSSHKeyDataSourceWithLister(lister), map[string]tftypes.Value{})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when neither slug nor name is set")
	}
	// Both slug and name.
	resp = readDS(t, internalprovider.NewSSHKeyDataSourceWithLister(lister), map[string]tftypes.Value{
		"slug": strVal("a"),
		"name": strVal("b"),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when both slug and name are set")
	}
}

// --- zcp_kubernetes_version ---

type fakeK8sVersionLister struct {
	versions []kubernetes.KubernetesVersion
}

func (f *fakeK8sVersionLister) ListVersions(_ context.Context) ([]kubernetes.KubernetesVersion, error) {
	return f.versions, nil
}

func TestKubernetesVersionDataSource_foundByVersion(t *testing.T) {
	lister := &fakeK8sVersionLister{
		versions: []kubernetes.KubernetesVersion{
			{ID: "kv-1", Slug: "k8s-132", Name: "Kubernetes 1.32", Version: "1.32.0", KubernetesClusterVersionID: "cv-9"},
		},
	}
	resp := readDS(t, internalprovider.NewKubernetesVersionDataSourceWithLister(lister), map[string]tftypes.Value{
		"version": strVal("1.32.0"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got struct {
		Slug             types.String `tfsdk:"slug"`
		Version          types.String `tfsdk:"version"`
		ID               types.String `tfsdk:"id"`
		Name             types.String `tfsdk:"name"`
		ClusterVersionID types.String `tfsdk:"cluster_version_id"`
		RegionID         types.String `tfsdk:"region_id"`
	}
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "kv-1" || got.Slug.ValueString() != "k8s-132" {
		t.Errorf("got id=%q slug=%q, want kv-1 / k8s-132", got.ID.ValueString(), got.Slug.ValueString())
	}
}

func TestKubernetesVersionDataSource_requiresExactlyOneLookup(t *testing.T) {
	lister := &fakeK8sVersionLister{}
	resp := readDS(t, internalprovider.NewKubernetesVersionDataSourceWithLister(lister), map[string]tftypes.Value{})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when neither slug nor version is set")
	}
	resp = readDS(t, internalprovider.NewKubernetesVersionDataSourceWithLister(lister), map[string]tftypes.Value{
		"slug":    strVal("a"),
		"version": strVal("b"),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when both slug and version are set")
	}
}
