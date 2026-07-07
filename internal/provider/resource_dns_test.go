package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/dns"

	internalprovider "github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

// fakeDNSService satisfies dnsServiceIface for both DNS resources.
type fakeDNSService struct {
	domain         *dns.Domain
	created        *dns.Domain
	recordResp     *dns.Domain
	err            error
	showErr        error
	deletedDomains []string
	deletedRecords []int
}

func (f *fakeDNSService) Show(_ context.Context, _ string) (*dns.Domain, error) {
	if f.showErr != nil {
		return nil, f.showErr
	}
	if f.domain == nil {
		return nil, &apierrors.APIError{StatusCode: 404, Message: "not found"}
	}
	return f.domain, f.err
}
func (f *fakeDNSService) Create(_ context.Context, _ dns.CreateDomainRequest) (*dns.Domain, error) {
	return f.created, f.err
}
func (f *fakeDNSService) Delete(_ context.Context, slug string) error {
	f.deletedDomains = append(f.deletedDomains, slug)
	if f.err == nil {
		f.domain = nil // subsequent Show returns 404 so pollUntilGone finishes
	}
	return f.err
}
func (f *fakeDNSService) CreateRecord(_ context.Context, _ string, _ dns.CreateRecordRequest) (*dns.Domain, error) {
	return f.recordResp, f.err
}
func (f *fakeDNSService) DeleteRecord(_ context.Context, _ string, recordID int) error {
	f.deletedRecords = append(f.deletedRecords, recordID)
	if f.err == nil && f.domain != nil {
		f.domain.Records = nil // record disappears from subsequent Shows
	}
	return f.err
}

// --- zcp_dns_domain ---

type dnsDomainStateModel struct {
	ID            types.String   `tfsdk:"id"`
	Name          types.String   `tfsdk:"name"`
	DNSProvider   types.String   `tfsdk:"dns_provider"`
	CloudProvider types.String   `tfsdk:"cloud_provider"`
	Region        types.String   `tfsdk:"region"`
	Project       types.String   `tfsdk:"project"`
	Status        types.Bool     `tfsdk:"status"`
	Timeouts      timeouts.Value `tfsdk:"timeouts"`
}

func dnsDomainSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewDNSDomainResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func dnsDomainRaw(t *testing.T, schResp resource.SchemaResponse, id, name string, status *bool) tftypes.Value {
	t.Helper()
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	str := func(v string) tftypes.Value {
		if v == "" {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, v)
	}
	statusVal := tftypes.NewValue(tftypes.Bool, nil)
	if status != nil {
		statusVal = tftypes.NewValue(tftypes.Bool, *status)
	}
	return tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             str(id),
		"name":           str(name),
		"dns_provider":   tftypes.NewValue(tftypes.String, nil),
		"cloud_provider": str("zsoftly"),
		"region":         str("yow-1"),
		"project":        tftypes.NewValue(tftypes.String, nil),
		"status":         statusVal,
		"timeouts":       timeoutsNull(t, schResp),
	})
}

func createDNSDomain(t *testing.T, svc *fakeDNSService, name string) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewDNSDomainResourceWithService(svc)
	schResp := dnsDomainSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	createReq := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: dnsDomainRaw(t, schResp, "", name, nil)},
	}
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)},
	}
	r.Create(context.Background(), createReq, createResp)
	return *createResp
}

func TestDNSDomainResource_createDefaultsDNSProvider(t *testing.T) {
	// dns_provider is Optional+Computed with a PowerDNS default; when config
	// omits it, Create must record the resolved value so state is never left
	// unknown and later refreshes cannot flip it to a different value.
	svc := &fakeDNSService{
		created: &dns.Domain{Slug: "example-com", Name: "example.com", Status: true},
	}
	resp := createDNSDomain(t, svc, "example.com")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var provider types.String
	if diags := resp.State.GetAttribute(context.Background(), path.Root("dns_provider"), &provider); diags.HasError() {
		t.Fatalf("reading dns_provider: %v", diags)
	}
	if provider.ValueString() != "PowerDNS" {
		t.Errorf("dns_provider = %q, want PowerDNS", provider.ValueString())
	}
}

func readDNSDomain(t *testing.T, svc *fakeDNSService, id string) resource.ReadResponse {
	t.Helper()
	r := internalprovider.NewDNSDomainResourceWithService(svc)
	schResp := dnsDomainSchema(t)
	active := true
	stateVal := dnsDomainRaw(t, schResp, id, "example.com", &active)
	readReq := resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), readReq, readResp)
	return *readResp
}

func deleteDNSDomain(t *testing.T, svc *fakeDNSService, id string) resource.DeleteResponse {
	t.Helper()
	r := internalprovider.NewDNSDomainResourceWithService(svc)
	schResp := dnsDomainSchema(t)
	active := true
	stateVal := dnsDomainRaw(t, schResp, id, "example.com", &active)
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	return deleteResp
}

func TestDNSDomainResource_createHappyPath(t *testing.T) {
	svc := &fakeDNSService{
		created: &dns.Domain{ID: "1", Slug: "example-com", Name: "example.com", Status: true},
	}
	resp := createDNSDomain(t, svc, "example.com")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got dnsDomainStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.ID.ValueString() != "example-com" {
		t.Errorf("ID = %q, want %q", got.ID.ValueString(), "example-com")
	}
	if !got.Status.ValueBool() {
		t.Error("Status = false, want true")
	}
}

func TestDNSDomainResource_createServiceError(t *testing.T) {
	svc := &fakeDNSService{err: errors.New("domain already exists")}
	resp := createDNSDomain(t, svc, "example.com")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func TestDNSDomainResource_readFound(t *testing.T) {
	svc := &fakeDNSService{
		domain: &dns.Domain{Slug: "example-com", Name: "example.com", DNSProvider: "PowerDNS", Status: true},
	}
	resp := readDNSDomain(t, svc, "example-com")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got dnsDomainStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Name.ValueString() != "example.com" {
		t.Errorf("Name = %q, want %q", got.Name.ValueString(), "example.com")
	}
	if got.DNSProvider.ValueString() != "PowerDNS" {
		t.Errorf("DNSProvider = %q, want %q", got.DNSProvider.ValueString(), "PowerDNS")
	}
}

func TestDNSDomainResource_readNotFound(t *testing.T) {
	svc := &fakeDNSService{showErr: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := readDNSDomain(t, svc, "example-com")
	if resp.Diagnostics.HasError() {
		t.Fatalf("read-not-found should not produce diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestDNSDomainResource_deleteHappyPath(t *testing.T) {
	svc := &fakeDNSService{
		domain: &dns.Domain{Slug: "example-com", Name: "example.com"},
	}
	resp := deleteDNSDomain(t, svc, "example-com")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if len(svc.deletedDomains) != 1 || svc.deletedDomains[0] != "example-com" {
		t.Errorf("Delete called with %v, want [example-com]", svc.deletedDomains)
	}
}

func TestDNSDomainResource_delete404IsNoOp(t *testing.T) {
	svc := &fakeDNSService{err: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := deleteDNSDomain(t, svc, "example-com")
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 on delete should be a no-op: %v", resp.Diagnostics)
	}
}

// --- zcp_dns_record ---

// fakeDNSRecordDeleter satisfies dnsRecordDeleter.
type fakeDNSRecordDeleter struct {
	deleted []string // "<type>/<fqdn>" per call
	err     error
	svc     *fakeDNSService // when set, removes matching records so pollUntilGone finishes
}

func (f *fakeDNSRecordDeleter) DeleteRecordByName(_ context.Context, _ string, fqdn, recType string) error {
	f.deleted = append(f.deleted, recType+"/"+fqdn)
	if f.err == nil && f.svc != nil && f.svc.domain != nil {
		var kept []dns.Record
		for _, rec := range f.svc.domain.Records {
			name := rec.Name
			if name != fqdn {
				kept = append(kept, rec)
			}
		}
		f.svc.domain.Records = kept
	}
	return f.err
}

type dnsRecordStateModel struct {
	ID       types.String   `tfsdk:"id"`
	Domain   types.String   `tfsdk:"domain"`
	Name     types.String   `tfsdk:"name"`
	Type     types.String   `tfsdk:"type"`
	Content  types.String   `tfsdk:"content"`
	TTL      types.Int64    `tfsdk:"ttl"`
	FQDN     types.String   `tfsdk:"fqdn"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func dnsRecordSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := internalprovider.NewDNSRecordResource()
	var schResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schResp)
	return schResp
}

func dnsRecordRaw(t *testing.T, schResp resource.SchemaResponse, id, domain, name, recType, content, fqdn string, ttl int64) tftypes.Value {
	t.Helper()
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	str := func(v string) tftypes.Value {
		if v == "" {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, v)
	}
	return tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":       str(id),
		"domain":   str(domain),
		"name":     str(name),
		"type":     str(recType),
		"content":  str(content),
		"ttl":      tftypes.NewValue(tftypes.Number, ttl),
		"fqdn":     str(fqdn),
		"timeouts": timeoutsNull(t, schResp),
	})
}

func createDNSRecord(t *testing.T, svc *fakeDNSService, deleter *fakeDNSRecordDeleter, domain, name, recType, content string, ttl int64) resource.CreateResponse {
	t.Helper()
	r := internalprovider.NewDNSRecordResourceWithService(svc, deleter)
	schResp := dnsRecordSchema(t)
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	createReq := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schResp.Schema, Raw: dnsRecordRaw(t, schResp, "", domain, name, recType, content, "", ttl)},
	}
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schResp.Schema, Raw: tftypes.NewValue(tfType, nil)},
	}
	r.Create(context.Background(), createReq, createResp)
	return *createResp
}

func TestDNSRecordResource_createResolvesFQDN(t *testing.T) {
	svc := &fakeDNSService{
		recordResp: &dns.Domain{Slug: "example-com"},
		domain: &dns.Domain{
			Slug: "example-com",
			Name: "example.com",
			Records: []dns.Record{
				{Name: "www.example.com.", Type: "A", TTL: 3600},
			},
		},
	}
	resp := createDNSRecord(t, svc, &fakeDNSRecordDeleter{}, "example-com", "www", "A", "192.0.2.10", 3600)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got dnsRecordStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.FQDN.ValueString() != "www.example.com." {
		t.Errorf("FQDN = %q, want www.example.com.", got.FQDN.ValueString())
	}
	if got.ID.ValueString() != "A/www.example.com." {
		t.Errorf("ID = %q, want A/www.example.com.", got.ID.ValueString())
	}
}

func TestDNSRecordResource_createServiceError(t *testing.T) {
	svc := &fakeDNSService{err: errors.New("invalid record")}
	resp := createDNSRecord(t, svc, &fakeDNSRecordDeleter{}, "example-com", "www", "A", "192.0.2.10", 3600)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on create failure, got none")
	}
}

func readDNSRecord(t *testing.T, svc *fakeDNSService, domain, name, recType string) resource.ReadResponse {
	t.Helper()
	r := internalprovider.NewDNSRecordResourceWithService(svc, &fakeDNSRecordDeleter{})
	schResp := dnsRecordSchema(t)
	stateVal := dnsRecordRaw(t, schResp, "A/www.example.com.", domain, name, recType, "192.0.2.10", "www.example.com.", 3600)
	readReq := resource.ReadRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	r.Read(context.Background(), readReq, readResp)
	return *readResp
}

func TestDNSRecordResource_readRefreshesTTL(t *testing.T) {
	svc := &fakeDNSService{
		domain: &dns.Domain{
			Slug: "example-com",
			Name: "example.com",
			Records: []dns.Record{
				{Name: "www.example.com.", Type: "A", TTL: 600},
			},
		},
	}
	resp := readDNSRecord(t, svc, "example-com", "www", "A")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var got dnsRecordStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.TTL.ValueInt64() != 600 {
		t.Errorf("TTL = %d, want 600", got.TTL.ValueInt64())
	}
	// content is write-only and must be preserved from state.
	if got.Content.ValueString() != "192.0.2.10" {
		t.Errorf("Content = %q, want preserved 192.0.2.10", got.Content.ValueString())
	}
}

func TestDNSRecordResource_readRecordGone(t *testing.T) {
	svc := &fakeDNSService{
		domain: &dns.Domain{Slug: "example-com", Name: "example.com"},
	}
	resp := readDNSRecord(t, svc, "example-com", "www", "A")
	if resp.Diagnostics.HasError() {
		t.Fatalf("read-not-found should not produce diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestDNSRecordResource_readDomainGone(t *testing.T) {
	svc := &fakeDNSService{showErr: &apierrors.APIError{StatusCode: 404, Message: "not found"}}
	resp := readDNSRecord(t, svc, "example-com", "www", "A")
	if resp.Diagnostics.HasError() {
		t.Fatalf("domain-gone read should not produce diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null after RemoveResource, got non-null")
	}
}

func TestDNSRecordResource_deleteByNameAndType(t *testing.T) {
	svc := &fakeDNSService{
		domain: &dns.Domain{
			Slug: "example-com",
			Name: "example.com",
			Records: []dns.Record{
				{Name: "www.example.com.", Type: "A", TTL: 3600},
			},
		},
	}
	deleter := &fakeDNSRecordDeleter{svc: svc}
	r := internalprovider.NewDNSRecordResourceWithService(svc, deleter)
	schResp := dnsRecordSchema(t)
	stateVal := dnsRecordRaw(t, schResp, "A/www.example.com.", "example-com", "www", "A", "192.0.2.10", "www.example.com.", 3600)
	deleteReq := resource.DeleteRequest{State: tfsdk.State{Schema: schResp.Schema, Raw: stateVal}}
	var deleteResp resource.DeleteResponse
	r.Delete(context.Background(), deleteReq, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", deleteResp.Diagnostics)
	}
	if len(deleter.deleted) != 1 || deleter.deleted[0] != "A/www.example.com." {
		t.Errorf("DeleteRecordByName called with %v, want [A/www.example.com.]", deleter.deleted)
	}
}
