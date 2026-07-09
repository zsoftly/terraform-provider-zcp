package provider_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/kubernetes"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeKubernetesService satisfies kubernetesServiceIface. getQueue lets a test
// return a sequence of clusters across successive Get calls (e.g. Scaling then
// Running); when exhausted, the last entry is returned for subsequent calls.
type fakeKubernetesService struct {
	created         *kubernetes.Cluster
	getQueue        []*kubernetes.Cluster
	createErr       error
	getErr          error
	scaleErr        error
	deleteErr       error
	upgradeErr      error
	versions        []kubernetes.KubernetesVersion
	scaledTo        []int
	deleted         []string
	planUpgrades    []string
	versionUpgrades []string
	operations      []string
	getCalls        int
}

func (f *fakeKubernetesService) Create(_ context.Context, _ kubernetes.CreateRequest) (*kubernetes.Cluster, error) {
	return f.created, f.createErr
}
func (f *fakeKubernetesService) Get(_ context.Context, _ string) (*kubernetes.Cluster, error) {
	f.getCalls++
	if f.getErr != nil {
		return nil, f.getErr
	}
	if len(f.getQueue) == 0 {
		return f.created, nil
	}
	idx := f.getCalls - 1
	if idx >= len(f.getQueue) {
		idx = len(f.getQueue) - 1
	}
	return f.getQueue[idx], nil
}
func (f *fakeKubernetesService) ListVersions(_ context.Context) ([]kubernetes.KubernetesVersion, error) {
	return f.versions, nil
}
func (f *fakeKubernetesService) Scale(_ context.Context, _ string, nodeSize int) error {
	f.scaledTo = append(f.scaledTo, nodeSize)
	f.operations = append(f.operations, "scale")
	return f.scaleErr
}
func (f *fakeKubernetesService) Upgrade(_ context.Context, _ string, req kubernetes.UpgradeRequest) error {
	f.planUpgrades = append(f.planUpgrades, req.Plan)
	f.operations = append(f.operations, "plan")
	return f.upgradeErr
}
func (f *fakeKubernetesService) UpgradeVersion(_ context.Context, _ string, req kubernetes.UpgradeVersionRequest) error {
	f.versionUpgrades = append(f.versionUpgrades, req.Slug)
	f.operations = append(f.operations, "version")
	return f.upgradeErr
}
func (f *fakeKubernetesService) Delete(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	return f.deleteErr
}

type kubernetesStateModel struct {
	ID              types.String   `tfsdk:"id"`
	Name            types.String   `tfsdk:"name"`
	CloudProvider   types.String   `tfsdk:"cloud_provider"`
	Region          types.String   `tfsdk:"region"`
	Version         types.String   `tfsdk:"version"`
	Plan            types.String   `tfsdk:"plan"`
	BillingCycle    types.String   `tfsdk:"billing_cycle"`
	Workers         types.Int64    `tfsdk:"workers"`
	StorageCategory types.String   `tfsdk:"storage_category"`
	Project         types.String   `tfsdk:"project"`
	SSHKey          types.String   `tfsdk:"ssh_key"`
	ControlNodes    types.Int64    `tfsdk:"control_nodes"`
	HA              types.Bool     `tfsdk:"ha"`
	Slug            types.String   `tfsdk:"slug"`
	State           types.String   `tfsdk:"state"`
	APIEndpoint     types.String   `tfsdk:"api_endpoint"`
	IPAddress       types.String   `tfsdk:"ip_address"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
}

func kubernetesSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewKubernetesClusterResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func kubernetesTFType(t *testing.T) tftypes.Type {
	t.Helper()
	return kubernetesSchema(t).Schema.Type().TerraformType(context.Background())
}

func kubernetesValues(t *testing.T, id string, workers int64) map[string]tftypes.Value {
	t.Helper()
	nullStr := func() tftypes.Value { return tftypes.NewValue(tftypes.String, nil) }
	idVal := nullStr()
	if id != "" {
		idVal = tftypes.NewValue(tftypes.String, id)
	}
	return map[string]tftypes.Value{
		"id":               idVal,
		"name":             tftypes.NewValue(tftypes.String, "k8s1"),
		"cloud_provider":   tftypes.NewValue(tftypes.String, "nimbo"),
		"region":           tftypes.NewValue(tftypes.String, "yow-1"),
		"version":          tftypes.NewValue(tftypes.String, "v1.36.1"),
		"plan":             tftypes.NewValue(tftypes.String, "k8s-li-yow-1"),
		"billing_cycle":    tftypes.NewValue(tftypes.String, "hourly"),
		"workers":          tftypes.NewValue(tftypes.Number, workers),
		"storage_category": tftypes.NewValue(tftypes.String, "pro-nvme"),
		"project":          nullStr(),
		"ssh_key":          tftypes.NewValue(tftypes.String, "mykey"),
		"control_nodes":    tftypes.NewValue(tftypes.Number, int64(1)),
		"ha":               tftypes.NewValue(tftypes.Bool, false),
		"slug":             idVal,
		"state":            nullStr(),
		"api_endpoint":     nullStr(),
		"ip_address":       nullStr(),
		"timeouts":         timeoutsNull(t, kubernetesSchema(t)),
	}
}

func createKubernetes(t *testing.T, svc *fakeKubernetesService, workers int64) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewKubernetesClusterResourceWithService(svc)
	schResp := kubernetesSchema(t)
	tfType := kubernetesTFType(t)
	planVal := tftypes.NewValue(tfType, kubernetesValues(t, "", workers))
	createReq := resource.CreateRequest{Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: planVal}}
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)},
	}
	r.Create(context.Background(), createReq, createResp)
	return *createResp
}

func TestKubernetesResource_createWaitsForRunning(t *testing.T) {
	// Get returns Running on the first poll so pollUntilReady returns without
	// hitting its 15s interval (keeping the unit test fast).
	svc := &fakeKubernetesService{
		created: &kubernetes.Cluster{Slug: "k8s1-abc", State: "Pending", NodeSize: 3},
		getQueue: []*kubernetes.Cluster{
			{Slug: "k8s1-abc", State: "Running", NodeSize: 3, ControlNodes: 1,
				Meta: &kubernetes.ClusterMeta{Endpoint: "https://10.0.0.9:6443", IPAddress: "10.0.0.9", Size: "3"}},
		},
	}
	resp := createKubernetes(t, svc, 3)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got kubernetesStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.State.ValueString() != "Running" {
		t.Errorf("State = %q, want Running", got.State.ValueString())
	}
	if got.APIEndpoint.ValueString() != "https://10.0.0.9:6443" {
		t.Errorf("APIEndpoint = %q", got.APIEndpoint.ValueString())
	}
	if got.IPAddress.ValueString() != "10.0.0.9" {
		t.Errorf("IPAddress = %q", got.IPAddress.ValueString())
	}
	if got.Workers.ValueInt64() != 3 {
		t.Errorf("Workers = %d, want 3", got.Workers.ValueInt64())
	}
}

func TestKubernetesResource_createServiceError(t *testing.T) {
	svc := &fakeKubernetesService{createErr: errors.New("quota exceeded")}
	resp := createKubernetes(t, svc, 3)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure")
	}
}

func TestKubernetesResource_createFailState(t *testing.T) {
	svc := &fakeKubernetesService{
		created:  &kubernetes.Cluster{Slug: "k8s1-abc", State: "Pending"},
		getQueue: []*kubernetes.Cluster{{Slug: "k8s1-abc", State: "Error"}},
	}
	resp := createKubernetes(t, svc, 3)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when cluster enters Error state")
	}
}

func TestKubernetesResource_updateScalesWorkers(t *testing.T) {
	svc := &fakeKubernetesService{
		getQueue: []*kubernetes.Cluster{
			{Slug: "k8s1-abc", State: "Running", NodeSize: 5, ControlNodes: 1, Meta: &kubernetes.ClusterMeta{Size: "5"}},
		},
	}
	r := internalprovider.NewKubernetesClusterResourceWithService(svc)
	schResp := kubernetesSchema(t)
	tfType := kubernetesTFType(t)
	stateVal := tftypes.NewValue(tfType, kubernetesValues(t, "k8s1-abc", 3))
	planVal := tftypes.NewValue(tfType, kubernetesValues(t, "k8s1-abc", 5))
	updateReq := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schResp.Schema, Raw: planVal},
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Update(context.Background(), updateReq, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", updateResp.Diagnostics)
	}
	if len(svc.scaledTo) != 1 || svc.scaledTo[0] != 5 {
		t.Errorf("Scale called with %v, want [5]", svc.scaledTo)
	}
	var got kubernetesStateModel
	if diags := updateResp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Workers.ValueInt64() != 5 {
		t.Errorf("Workers = %d, want 5", got.Workers.ValueInt64())
	}
}

func TestKubernetesResource_updateUpgradesVersionAndPlanInPlace(t *testing.T) {
	restore := internalprovider.SetKubernetesPollIntervalForTest(time.Millisecond)
	defer restore()

	svc := &fakeKubernetesService{
		getQueue: []*kubernetes.Cluster{
			{Slug: "k8s1-abc", State: "Running", Version: "v1.36.1", RegionID: "region-yow", NodeSize: 3},
			{Slug: "k8s1-abc", State: "Running", Version: "v1.37.0", RegionID: "region-yow", NodeSize: 3},
			{Slug: "k8s1-abc", State: "Running", Version: "v1.37.0", RegionID: "region-yow", NodeSize: 3},
			{Slug: "k8s1-abc", State: "Running", Version: "v1.37.0", RegionID: "region-yow", NodeSize: 3},
		},
		versions: []kubernetes.KubernetesVersion{
			{Slug: "v1370-yow", Version: "v1.37.0", RegionID: "region-yow", KubernetesClusterVersionID: "cluster-version-137"},
		},
	}
	r := internalprovider.NewKubernetesClusterResourceWithService(svc)
	schResp := kubernetesSchema(t)
	tfType := kubernetesTFType(t)
	stateVal := tftypes.NewValue(tfType, kubernetesValues(t, "k8s1-abc", 3))
	planValues := kubernetesValues(t, "k8s1-abc", 3)
	planValues["version"] = tftypes.NewValue(tftypes.String, "v1.37.0")
	planValues["plan"] = tftypes.NewValue(tftypes.String, "k8s-hi-yow-1")
	planVal := tftypes.NewValue(tfType, planValues)
	updateReq := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schResp.Schema, Raw: planVal},
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Update(context.Background(), updateReq, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", updateResp.Diagnostics)
	}
	if len(svc.versionUpgrades) != 1 || svc.versionUpgrades[0] != "v1370-yow" {
		t.Errorf("versionUpgrades = %v, want [v1370-yow]", svc.versionUpgrades)
	}
	if len(svc.planUpgrades) != 1 || svc.planUpgrades[0] != "k8s-hi-yow-1" {
		t.Errorf("planUpgrades = %v, want [k8s-hi-yow-1]", svc.planUpgrades)
	}
	if len(svc.operations) != 2 || svc.operations[0] != "version" || svc.operations[1] != "plan" {
		t.Errorf("operations = %v, want [version plan]", svc.operations)
	}
	// No worker change → no scaling.
	if len(svc.scaledTo) != 0 {
		t.Errorf("Scale should not be called, got %v", svc.scaledTo)
	}
}

func TestKubernetesResource_updateWaitsBeforeScaleAfterPlanUpgrade(t *testing.T) {
	restore := internalprovider.SetKubernetesPollIntervalForTest(time.Millisecond)
	defer restore()

	svc := &fakeKubernetesService{
		getQueue: []*kubernetes.Cluster{
			{Slug: "k8s1-abc", State: "Upgrading", Version: "v1.36.1", NodeSize: 3},
			{Slug: "k8s1-abc", State: "Running", Version: "v1.36.1", NodeSize: 3},
			{Slug: "k8s1-abc", State: "Running", Version: "v1.36.1", NodeSize: 5, Meta: &kubernetes.ClusterMeta{Size: "5"}},
		},
	}
	r := internalprovider.NewKubernetesClusterResourceWithService(svc)
	schResp := kubernetesSchema(t)
	tfType := kubernetesTFType(t)
	stateVal := tftypes.NewValue(tfType, kubernetesValues(t, "k8s1-abc", 3))
	planValues := kubernetesValues(t, "k8s1-abc", 5)
	planValues["plan"] = tftypes.NewValue(tftypes.String, "k8s-hi-yow-1")
	planVal := tftypes.NewValue(tfType, planValues)
	updateReq := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schResp.Schema, Raw: planVal},
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Update(context.Background(), updateReq, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", updateResp.Diagnostics)
	}
	if len(svc.operations) != 2 || svc.operations[0] != "plan" || svc.operations[1] != "scale" {
		t.Errorf("operations = %v, want [plan scale]", svc.operations)
	}
}

func TestKubernetesResource_updateNoWorkerChangeSkipsScale(t *testing.T) {
	svc := &fakeKubernetesService{}
	r := internalprovider.NewKubernetesClusterResourceWithService(svc)
	schResp := kubernetesSchema(t)
	tfType := kubernetesTFType(t)
	stateVal := tftypes.NewValue(tfType, kubernetesValues(t, "k8s1-abc", 3))
	planVal := tftypes.NewValue(tfType, kubernetesValues(t, "k8s1-abc", 3))
	updateReq := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schResp.Schema, Raw: planVal},
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Update(context.Background(), updateReq, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", updateResp.Diagnostics)
	}
	if len(svc.scaledTo) != 0 {
		t.Errorf("Scale should not be called when workers unchanged, got %v", svc.scaledTo)
	}
}

func readKubernetes(t *testing.T, svc *fakeKubernetesService, slug string) resource.ReadResponse {
	t.Helper()
	r := internalprovider.NewKubernetesClusterResourceWithService(svc)
	schResp := kubernetesSchema(t)
	tfType := kubernetesTFType(t)
	stateVal := tftypes.NewValue(tfType, kubernetesValues(t, slug, 3))
	readReq := resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), readReq, readResp)
	return *readResp
}

func TestKubernetesResource_readFound(t *testing.T) {
	svc := &fakeKubernetesService{
		created: &kubernetes.Cluster{Slug: "k8s1-abc", Name: "k8s1", State: "Running", NodeSize: 3, ControlNodes: 1},
	}
	resp := readKubernetes(t, svc, "k8s1-abc")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got kubernetesStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.State.ValueString() != "Running" {
		t.Errorf("State = %q, want Running", got.State.ValueString())
	}
}

func TestKubernetesResource_readNotFound(t *testing.T) {
	svc := &fakeKubernetesService{getErr: &apierrors.APIError{StatusCode: 404}}
	resp := readKubernetes(t, svc, "gone")
	if resp.Diagnostics.HasError() {
		t.Fatalf("not-found read should not error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected null state after RemoveResource")
	}
}

func deleteKubernetes(t *testing.T, svc *fakeKubernetesService, slug string) resource.DeleteResponse {
	t.Helper()
	r := internalprovider.NewKubernetesClusterResourceWithService(svc)
	schResp := kubernetesSchema(t)
	tfType := kubernetesTFType(t)
	stateVal := tftypes.NewValue(tfType, kubernetesValues(t, slug, 3))
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

func TestKubernetesResource_deleteHappyPath(t *testing.T) {
	svc := &fakeKubernetesService{getErr: &apierrors.APIError{StatusCode: 404}}
	resp := deleteKubernetes(t, svc, "k8s1-abc")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "k8s1-abc" {
		t.Errorf("Delete called with %v, want [k8s1-abc]", svc.deleted)
	}
}
