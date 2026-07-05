package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/ipaddress"
	"github.com/zsoftly/zcp-cli/pkg/httpclient"
)

var _ resource.Resource = &ipAssociationResource{}
var _ resource.ResourceWithConfigure = &ipAssociationResource{}
var _ resource.ResourceWithImportState = &ipAssociationResource{}

// ipAssociationServiceIface associates/disassociates an owned IP with a VM via
// static NAT. The relationship works for both VPC tiers and standalone networks
// — the static-NAT API takes the network the VM is on, not a VPC.
type ipAssociationServiceIface interface {
	Enable(ctx context.Context, ipSlug, vmSlug, networkSlug string) error
	Disable(ctx context.Context, ipSlug string) error
	List(ctx context.Context) ([]ipaddress.IPAddress, error)
}

// ipAssociationService talks to the static-NAT endpoint directly. The released
// CLI's EnableStaticNAT sends only {virtual_machine}, but the API requires
// {virtual_machine, network} (it rejects the CLI body with "The network field is
// required"), and there is no Disable in the CLI package — disable is a DELETE on
// the same endpoint. So both calls go through the shared HTTP client.
type ipAssociationService struct {
	client *httpclient.Client
	ipSvc  *ipaddress.Service
}

func (s *ipAssociationService) Enable(ctx context.Context, ipSlug, vmSlug, networkSlug string) error {
	body := map[string]string{"virtual_machine": vmSlug, "network": networkSlug}
	var resp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := s.client.Post(ctx, "/ipaddresses/"+ipSlug+"/static-nat", body, &resp); err != nil {
		return err
	}
	if resp.Status != "" && !strings.EqualFold(resp.Status, "Success") {
		return fmt.Errorf("enabling static NAT: %s", resp.Message)
	}
	return nil
}

func (s *ipAssociationService) Disable(ctx context.Context, ipSlug string) error {
	return s.client.Delete(ctx, "/ipaddresses/"+ipSlug+"/static-nat", nil)
}

func (s *ipAssociationService) List(ctx context.Context) ([]ipaddress.IPAddress, error) {
	return s.ipSvc.List(ctx, "", "", "")
}

type ipAssociationResource struct {
	svc ipAssociationServiceIface
}

type ipAssociationResourceModel struct {
	ID             types.String `tfsdk:"id"`
	IPAddress      types.String `tfsdk:"ip_address"`
	VirtualMachine types.String `tfsdk:"virtual_machine"`
	Network        types.String `tfsdk:"network"`
}

func NewIPAssociationResource() resource.Resource {
	return &ipAssociationResource{}
}

func (r *ipAssociationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ip_association"
}

func (r *ipAssociationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Associates an owned public IP (`zcp_ip_address`) with an instance via static NAT — the AWS `aws_eip_association` / Azure public-IP-association analog. The `network` is the network the instance is on (a VPC tier or a standalone network). Association is create/delete only; changing any field forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Association ID (the IP slug; an IP has at most one static-NAT association).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"ip_address": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Slug of the owned `zcp_ip_address` to associate. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"virtual_machine": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Slug of the `zcp_instance` to associate the IP with. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"network": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Slug of the network the instance is on (a VPC tier or a standalone `zcp_network`). Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
		},
	}
}

func (r *ipAssociationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = &ipAssociationService{client: pd.Client, ipSvc: ipaddress.NewService(pd.Client)}
}

func (r *ipAssociationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model ipAssociationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_ip_association cannot be created: bearer_token is missing.")
		return
	}

	if err := r.svc.Enable(ctx, model.IPAddress.ValueString(), model.VirtualMachine.ValueString(), model.Network.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to associate IP", err.Error())
		return
	}
	model.ID = model.IPAddress
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *ipAssociationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model ipAssociationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_ip_association cannot be read: bearer_token is missing.")
		return
	}

	ips, err := r.svc.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read IP association", err.Error())
		return
	}
	slug := model.IPAddress.ValueString()
	for _, ip := range ips {
		if ip.Slug == slug {
			// The IP exists; the association is gone if it no longer points at a VM.
			if ip.VirtualMachineID == "" {
				resp.State.RemoveResource(ctx)
				return
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	// IP released out of band.
	resp.State.RemoveResource(ctx)
}

// Update never runs with a real diff: every attribute is RequiresReplace. It
// exists to satisfy the resource interface.
func (r *ipAssociationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model ipAssociationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *ipAssociationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model ipAssociationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_ip_association cannot be deleted: bearer_token is missing.")
		return
	}
	if err := r.svc.Disable(ctx, model.IPAddress.ValueString()); err != nil &&
		!apierrors.IsNotFound(err) && !apierrors.IsResourceNotFound(err) {
		resp.Diagnostics.AddError("Failed to disassociate IP", err.Error())
	}
}

// ImportState accepts "<ip-slug>/<vm-slug>/<network-slug>".
func (r *ipAssociationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 3)
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected \"<ip-slug>/<vm-slug>/<network-slug>\"")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip_address"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("virtual_machine"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("network"), parts[2])...)
}
