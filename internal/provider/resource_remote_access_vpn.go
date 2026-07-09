package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/ipaddress"
)

var _ resource.Resource = &remoteAccessVPNResource{}
var _ resource.ResourceWithConfigure = &remoteAccessVPNResource{}
var _ resource.ResourceWithImportState = &remoteAccessVPNResource{}

type remoteAccessVPNServiceIface interface {
	ListRemoteAccessVPNs(ctx context.Context, ipSlug string) ([]ipaddress.RemoteAccessVPN, error)
	EnableRemoteAccessVPN(ctx context.Context, ipSlug string) (*ipaddress.RemoteAccessVPN, error)
	DisableRemoteAccessVPN(ctx context.Context, ipSlug, vpnID string) error
}

type remoteAccessVPNResource struct {
	svc remoteAccessVPNServiceIface
}

type remoteAccessVPNResourceModel struct {
	ID        types.String   `tfsdk:"id"`
	IPAddress types.String   `tfsdk:"ip_address"`
	PublicIP  types.String   `tfsdk:"public_ip"`
	State     types.String   `tfsdk:"state"`
	Timeouts  timeouts.Value `tfsdk:"timeouts"`
}

func NewRemoteAccessVPNResource() resource.Resource {
	return &remoteAccessVPNResource{}
}

func (r *remoteAccessVPNResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_remote_access_vpn"
}

func (r *remoteAccessVPNResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Enables remote access VPN on a public IP address. Pair with `zcp_vpn_user` for the accounts allowed to connect. Destroy disables the VPN on the IP.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Remote access VPN ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"ip_address": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Public IP address slug the VPN is enabled on. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"public_ip": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Public IP clients connect to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current state of the remote access VPN.",
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

func (r *remoteAccessVPNResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = ipaddress.NewService(pd.Client)
}

func (r *remoteAccessVPNResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model remoteAccessVPNResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_remote_access_vpn cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	ipSlug := model.IPAddress.ValueString()
	vpn, err := r.svc.EnableRemoteAccessVPN(ctx, ipSlug)
	if err != nil {
		resp.Diagnostics.AddError("Failed to enable remote access VPN", err.Error())
		return
	}

	if vpn == nil || vpn.ID == "" {
		// Resolve from the list when the enable response omits the VPN body.
		vpns, lerr := r.svc.ListRemoteAccessVPNs(ctx, ipSlug)
		if lerr != nil {
			resp.Diagnostics.AddError("Failed to resolve enabled remote access VPN", lerr.Error())
			return
		}
		if len(vpns) == 0 {
			resp.Diagnostics.AddError(
				"Failed to resolve enabled remote access VPN",
				fmt.Sprintf("the VPN was enabled on %q but does not appear in the VPN list.", ipSlug),
			)
			return
		}
		vpn = &vpns[0]
	}

	model.ID = types.StringValue(vpn.ID)
	model.PublicIP = stringOrNull(vpn.PublicIP)
	model.State = stringOrNull(vpn.State)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// stringOrNull maps an empty API string to a null state value.
func stringOrNull(v string) types.String {
	if v == "" {
		return types.StringNull()
	}
	return types.StringValue(v)
}

func (r *remoteAccessVPNResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model remoteAccessVPNResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_remote_access_vpn cannot be read: bearer_token is missing.")
		return
	}

	vpns, err := r.svc.ListRemoteAccessVPNs(ctx, model.IPAddress.ValueString())
	if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read remote access VPN", err.Error())
		return
	}

	id := model.ID.ValueString()
	for _, vpn := range vpns {
		if vpn.ID == id {
			model.PublicIP = stringOrNull(vpn.PublicIP)
			model.State = stringOrNull(vpn.State)
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *remoteAccessVPNResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are ForceNew; Terraform never invokes this method.
}

func (r *remoteAccessVPNResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model remoteAccessVPNResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_remote_access_vpn cannot be deleted: bearer_token is missing.")
		return
	}

	deleteTimeout, diags := model.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	ipSlug := model.IPAddress.ValueString()
	vpnID := model.ID.ValueString()
	err := r.svc.DisableRemoteAccessVPN(deleteCtx, ipSlug, vpnID)
	if err != nil && !apierrors.IsNotFound(err) && !apierrors.IsResourceNotFound(err) {
		resp.Diagnostics.AddError("Failed to disable remote access VPN", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		vpns, err := r.svc.ListRemoteAccessVPNs(ctx, ipSlug)
		if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for _, vpn := range vpns {
			if vpn.ID == vpnID {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError("Remote access VPN disable did not complete", err.Error())
	}
}

func (r *remoteAccessVPNResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Expected format: "ip-slug/vpn-id"
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected format \"<ip_address>/<vpn_id>\", got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip_address"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
