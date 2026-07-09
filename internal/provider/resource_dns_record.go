package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/dns"
)

var _ resource.Resource = &dnsRecordResource{}
var _ resource.ResourceWithConfigure = &dnsRecordResource{}
var _ resource.ResourceWithImportState = &dnsRecordResource{}

// dnsRecordDeleter removes an RRset by name and type. The live DNS API models
// records as PowerDNS RRsets without numeric IDs (verified 2026-07-05), so
// deletion goes through the SDK's DeleteRecordByName rather than the
// record_id-based DeleteRecord, which cannot work against the live API.
// *dns.Service satisfies this interface as of zcp-cli v0.0.22.
type dnsRecordDeleter interface {
	DeleteRecordByName(ctx context.Context, domainSlug, fqdn, recType string) error
}

type dnsRecordResource struct {
	svc     dnsServiceIface
	deleter dnsRecordDeleter
}

type dnsRecordResourceModel struct {
	ID       types.String   `tfsdk:"id"`
	Domain   types.String   `tfsdk:"domain"`
	Name     types.String   `tfsdk:"name"`
	Type     types.String   `tfsdk:"type"`
	Content  types.String   `tfsdk:"content"`
	TTL      types.Int64    `tfsdk:"ttl"`
	FQDN     types.String   `tfsdk:"fqdn"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func NewDNSRecordResource() resource.Resource {
	return &dnsRecordResource{}
}

func (r *dnsRecordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_record"
}

func (r *dnsRecordResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a DNS record set in a `zcp_dns_domain`. The DNS backend models records as " +
			"record sets identified by name and type, so declare one resource per name/type pair. Every change " +
			"forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Synthetic record identifier (`<type>/<fqdn>`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"domain": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Parent DNS domain slug. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Relative record name (e.g. `www`), or `@` for the zone apex. The zone is appended by the backend. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record type: `A`, `AAAA`, `CNAME`, `MX`, `TXT`, `NS`, or `SRV`. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"content": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record content (e.g. an IPv4 address for `A`). Write-only: the API does not return content in a comparable form, so out-of-band content changes are not detected. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"ttl": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Time to live in seconds (e.g. `3600`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"fqdn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Fully qualified record name as stored by the backend (e.g. `www.example.com.`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Delete: true,
			}),
		},
	}
}

func (r *dnsRecordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	svc := dns.NewService(pd.Client)
	r.svc = svc
	r.deleter = svc
}

// matchRecord reports whether a stored record matches the given fqdn and type.
func matchRecord(rec dns.Record, fqdn, recType string) bool {
	name := strings.ToLower(rec.Name)
	if !strings.HasSuffix(name, ".") {
		name += "."
	}
	return name == fqdn && strings.EqualFold(rec.Type, recType)
}

func (r *dnsRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model dnsRecordResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_dns_record cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	domainSlug := model.Domain.ValueString()
	recType := model.Type.ValueString()

	if _, err := r.svc.CreateRecord(ctx, domainSlug, dns.CreateRecordRequest{
		Name:    model.Name.ValueString(),
		Type:    recType,
		Content: model.Content.ValueString(),
		TTL:     int(model.TTL.ValueInt64()),
	}); err != nil {
		resp.Diagnostics.AddError("Failed to create DNS record", err.Error())
		return
	}

	// Resolve the stored record set: the backend appends the zone to the
	// relative name, so compute the FQDN from the domain and poll until the
	// record set is visible.
	var fqdn string
	if err := pollUntilReady(ctx, 5*time.Second, func(ctx context.Context) (bool, error) {
		domain, serr := r.svc.Show(ctx, domainSlug)
		if serr != nil {
			return false, serr
		}
		fqdn = dns.CanonicalRecordFQDN(model.Name.ValueString(), domain.Name)
		for _, rec := range domain.Records {
			if matchRecord(rec, fqdn, recType) {
				if rec.TTL != 0 {
					model.TTL = types.Int64Value(int64(rec.TTL))
				}
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		// The create was accepted, so persist a partial state before erroring:
		// Terraform then records the resource as tainted and the next apply
		// destroys and recreates it instead of orphaning the record. Delete
		// resolves a null fqdn from the domain on its own.
		if fqdn != "" {
			model.FQDN = types.StringValue(fqdn)
			model.ID = types.StringValue(strings.ToUpper(recType) + "/" + fqdn)
		} else {
			model.FQDN = types.StringNull()
			model.ID = types.StringValue(strings.ToUpper(recType) + "/" + strings.ToLower(model.Name.ValueString()))
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
		resp.Diagnostics.AddError(
			"DNS record did not appear after create",
			fmt.Sprintf("record %s %s on domain %s was accepted but never showed up in the zone: %s. The resource is recorded as tainted, so the next apply replaces it.", recType, model.Name.ValueString(), domainSlug, err),
		)
		return
	}

	model.FQDN = types.StringValue(fqdn)
	model.ID = types.StringValue(strings.ToUpper(recType) + "/" + fqdn)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *dnsRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model dnsRecordResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_dns_record cannot be read: bearer_token is missing.")
		return
	}

	domain, err := r.svc.Show(ctx, model.Domain.ValueString())
	if isBackendNotFound(err) {
		// Parent domain is gone, so the record is too.
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read DNS record", err.Error())
		return
	}

	recType := model.Type.ValueString()
	fqdn := dns.CanonicalRecordFQDN(model.Name.ValueString(), domain.Name)
	for _, rec := range domain.Records {
		if matchRecord(rec, fqdn, recType) {
			if rec.TTL != 0 {
				model.TTL = types.Int64Value(int64(rec.TTL))
			}
			// content is write-only: the API returns record contents in a
			// backend-specific shape the SDK does not surface.
			model.FQDN = types.StringValue(fqdn)
			model.ID = types.StringValue(strings.ToUpper(recType) + "/" + fqdn)
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *dnsRecordResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are ForceNew; Terraform never invokes this method.
}

func (r *dnsRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model dnsRecordResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil || r.deleter == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_dns_record cannot be deleted: bearer_token is missing.")
		return
	}

	deleteTimeout, diags := model.Timeouts.Delete(ctx, 2*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	domainSlug := model.Domain.ValueString()
	recType := model.Type.ValueString()
	fqdn := model.FQDN.ValueString()
	if fqdn == "" {
		// State written before fqdn existed (or import without a read): derive
		// it from the domain.
		domain, serr := r.svc.Show(deleteCtx, domainSlug)
		if isBackendNotFound(serr) {
			return // domain gone → record gone
		}
		if serr != nil {
			resp.Diagnostics.AddError("Failed to resolve DNS record for delete", serr.Error())
			return
		}
		fqdn = dns.CanonicalRecordFQDN(model.Name.ValueString(), domain.Name)
	}

	err := r.deleter.DeleteRecordByName(deleteCtx, domainSlug, fqdn, recType)
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete DNS record", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		domain, err := r.svc.Show(ctx, domainSlug)
		if isBackendNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for _, rec := range domain.Records {
			if matchRecord(rec, fqdn, recType) {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError("DNS record deletion did not complete", err.Error())
	}
}

// ImportState seeds the record's identity. Format:
//
//	<domain-slug>/<type>/<relative-name>
//
// content cannot be imported (the API does not return it); set it in config
// before importing to avoid a replacement plan. ttl and fqdn come from the
// subsequent Read.
func (r *dnsRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"domain", "type", "name"}
	importPositional(ctx, req, resp, fields, 3, "<domain>/<type>/<name>")
}
