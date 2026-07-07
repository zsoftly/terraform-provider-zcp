package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/iso"
	"github.com/zsoftly/zcp-cli/pkg/api/template"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// --- zcp_iso ---

type fakeISOService struct {
	isos      []iso.ISO
	created   *iso.ISO
	createReq iso.CreateRequest
	updates   []iso.UpdateRequest
	err       error
	deleted   []string
}

func (f *fakeISOService) List(_ context.Context, _ string) ([]iso.ISO, error) {
	return f.isos, f.err
}
func (f *fakeISOService) Create(_ context.Context, req iso.CreateRequest) (*iso.ISO, error) {
	f.createReq = req
	return f.created, f.err
}
func (f *fakeISOService) Update(_ context.Context, _ string, req iso.UpdateRequest) error {
	f.updates = append(f.updates, req)
	return f.err
}
func (f *fakeISOService) Delete(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	if f.err == nil {
		f.isos = nil
	}
	return f.err
}

func isoConfig(id string) map[string]tftypes.Value {
	cfg := map[string]tftypes.Value{
		"name":           strVal("rescue-iso"),
		"url":            strVal("https://mirror.example.com/rescue.iso"),
		"cloud_provider": strVal("zsoftly"),
		"region":         strVal("yow-1"),
		"billing_cycle":  strVal("hourly"),
		// The framework applies the schema default at plan time; these tests
		// invoke the resource directly, so the defaulted value is set here.
		"is_bootable": tftypes.NewValue(tftypes.Bool, true),
	}
	if id != "" {
		cfg["id"] = strVal(id)
	}
	return cfg
}

func TestISOResource_createHappyPath(t *testing.T) {
	svc := &fakeISOService{
		created: &iso.ISO{Slug: "rescue-iso-i1", Name: "rescue-iso", State: "Ready"},
	}
	r := internalprovider.NewISOResourceWithService(svc)
	resp := runCreate(t, r, isoConfig(""))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := stateID(t, resp.State); got != "rescue-iso-i1" {
		t.Errorf("ID = %q, want rescue-iso-i1", got)
	}
	// is_bootable carries a schema default of true (simulated in isoConfig).
	if !svc.createReq.IsBootable {
		t.Error("createReq.IsBootable = false, want true (default)")
	}
	if svc.createReq.ImageType != "ISO" {
		t.Errorf("createReq.ImageType = %q, want ISO", svc.createReq.ImageType)
	}
}

func TestISOResource_createServiceError(t *testing.T) {
	svc := &fakeISOService{err: errors.New("bad url")}
	r := internalprovider.NewISOResourceWithService(svc)
	resp := runCreate(t, r, isoConfig(""))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func TestISOResource_updatePermissions(t *testing.T) {
	svc := &fakeISOService{}
	r := internalprovider.NewISOResourceWithService(svc)
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)

	stateCfg := isoConfig("rescue-iso-i1")
	planCfg := isoConfig("rescue-iso-i1")
	planCfg["is_extractable"] = tftypes.NewValue(tftypes.Bool, true)

	_, stateVal := rawFor(t, r, stateCfg)
	_, planVal := rawFor(t, r, planCfg)
	updateReq := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schResp.Schema, Raw: planVal},
		State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal},
	}
	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Update(context.Background(), updateReq, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", updateResp.Diagnostics)
	}
	if len(svc.updates) != 1 || !svc.updates[0].IsExtractable || !svc.updates[0].IsBootable {
		t.Errorf("updates = %+v, want one with IsExtractable+IsBootable true", svc.updates)
	}
}

func TestISOResource_readNotFoundRemoves(t *testing.T) {
	svc := &fakeISOService{}
	r := internalprovider.NewISOResourceWithService(svc)
	resp := runRead(t, r, isoConfig("rescue-iso-i1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestISOResource_deleteHappyPath(t *testing.T) {
	svc := &fakeISOService{
		isos: []iso.ISO{{Slug: "rescue-iso-i1"}},
	}
	r := internalprovider.NewISOResourceWithService(svc)
	resp := runDelete(t, r, isoConfig("rescue-iso-i1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "rescue-iso-i1" {
		t.Errorf("Delete called with %v, want [rescue-iso-i1]", svc.deleted)
	}
}

// --- zcp_account_template ---

type fakeAccountTemplateService struct {
	templates []template.AccountTemplate
	created   *template.AccountTemplate
	createReq template.CreateAccountTemplateRequest
	err       error
	deleted   []string
}

func (f *fakeAccountTemplateService) ListAccount(_ context.Context) ([]template.AccountTemplate, error) {
	return f.templates, f.err
}
func (f *fakeAccountTemplateService) CreateAccount(_ context.Context, req template.CreateAccountTemplateRequest) (*template.AccountTemplate, error) {
	f.createReq = req
	return f.created, f.err
}
func (f *fakeAccountTemplateService) DeleteAccount(_ context.Context, slug string) error {
	f.deleted = append(f.deleted, slug)
	if f.err == nil {
		f.templates = nil
	}
	return f.err
}

func accountTemplateConfig(id string) map[string]tftypes.Value {
	cfg := map[string]tftypes.Value{
		"name":            strVal("golden-web"),
		"virtual_machine": strVal("vm1-abc"),
		"cloud_provider":  strVal("zsoftly"),
		"region":          strVal("yow-1"),
		"billing_cycle":   strVal("monthly"),
	}
	if id != "" {
		cfg["id"] = strVal(id)
	}
	return cfg
}

func TestAccountTemplateResource_createFromVM(t *testing.T) {
	svc := &fakeAccountTemplateService{
		created: &template.AccountTemplate{Slug: "golden-web-t1", Name: "golden-web", State: "Ready"},
	}
	r := internalprovider.NewAccountTemplateResourceWithService(svc)
	resp := runCreate(t, r, accountTemplateConfig(""))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := stateID(t, resp.State); got != "golden-web-t1" {
		t.Errorf("ID = %q, want golden-web-t1", got)
	}
	if svc.createReq.VirtualMachine != "vm1-abc" {
		t.Errorf("createReq.VirtualMachine = %q, want vm1-abc", svc.createReq.VirtualMachine)
	}
}

func TestAccountTemplateResource_createServiceError(t *testing.T) {
	svc := &fakeAccountTemplateService{err: errors.New("capture failed")}
	r := internalprovider.NewAccountTemplateResourceWithService(svc)
	resp := runCreate(t, r, accountTemplateConfig(""))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func TestAccountTemplateResource_readNotFoundRemoves(t *testing.T) {
	svc := &fakeAccountTemplateService{}
	r := internalprovider.NewAccountTemplateResourceWithService(svc)
	resp := runRead(t, r, accountTemplateConfig("golden-web-t1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestAccountTemplateResource_deleteHappyPath(t *testing.T) {
	svc := &fakeAccountTemplateService{
		templates: []template.AccountTemplate{{Slug: "golden-web-t1"}},
	}
	r := internalprovider.NewAccountTemplateResourceWithService(svc)
	resp := runDelete(t, r, accountTemplateConfig("golden-web-t1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != "golden-web-t1" {
		t.Errorf("DeleteAccount called with %v, want [golden-web-t1]", svc.deleted)
	}
}

func TestAccountTemplateResource_validateConfigSource(t *testing.T) {
	r := internalprovider.NewAccountTemplateResourceWithService(nil).(resource.ResourceWithValidateConfig)
	var schResp resource.SchemaResponse
	r.(resource.Resource).Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	tfType := schResp.Schema.Type().TerraformType(context.Background())

	run := func(mutate func(map[string]tftypes.Value)) resource.ValidateConfigResponse {
		raw := accountTemplateConfig("")
		_, full := rawFor(t, r.(resource.Resource), raw)
		vals := map[string]tftypes.Value{}
		if err := full.As(&vals); err != nil {
			t.Fatalf("decomposing raw: %v", err)
		}
		mutate(vals)
		req := resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, vals)}}
		var resp resource.ValidateConfigResponse
		r.ValidateConfig(context.Background(), req, &resp)
		return resp
	}

	// accountTemplateConfig sets virtual_machine only: the valid single-source case.
	if resp := run(func(_ map[string]tftypes.Value) {}); resp.Diagnostics.HasError() {
		t.Errorf("virtual_machine only: unexpected error: %v", resp.Diagnostics)
	}
	if resp := run(func(v map[string]tftypes.Value) {
		v["url"] = tftypes.NewValue(tftypes.String, "https://mirror.example.com/img.qcow2")
	}); !resp.Diagnostics.HasError() {
		t.Error("both url and virtual_machine set: want error, got none")
	}
	if resp := run(func(v map[string]tftypes.Value) {
		v["virtual_machine"] = tftypes.NewValue(tftypes.String, nil)
	}); !resp.Diagnostics.HasError() {
		t.Error("neither url nor virtual_machine set: want error, got none")
	}
	// An unknown value resolves at apply time, so validation must not fail.
	if resp := run(func(v map[string]tftypes.Value) {
		v["url"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	}); resp.Diagnostics.HasError() {
		t.Errorf("unknown url: unexpected error: %v", resp.Diagnostics)
	}
}
