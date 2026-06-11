package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/vpn"
)

var _ resource.Resource = &vpnCustomerGatewayResource{}
var _ resource.ResourceWithConfigure = &vpnCustomerGatewayResource{}
var _ resource.ResourceWithImportState = &vpnCustomerGatewayResource{}

type vpnCustomerGatewayServiceIface interface {
	List(ctx context.Context) ([]vpn.CustomerGateway, error)
	Get(ctx context.Context, slug string) (*vpn.CustomerGateway, error)
	Create(ctx context.Context, req vpn.CustomerGatewayRequest) (*vpn.CustomerGateway, error)
	Update(ctx context.Context, slug string, req vpn.CustomerGatewayRequest) (*vpn.CustomerGateway, error)
	Delete(ctx context.Context, slug string) error
}

type vpnCustomerGatewayResource struct {
	svc            vpnCustomerGatewayServiceIface
	defaultProject string
}

type vpnCustomerGatewayResourceModel struct {
	ID                 types.String   `tfsdk:"id"`
	Name               types.String   `tfsdk:"name"`
	Gateway            types.String   `tfsdk:"gateway"`
	CIDRList           types.String   `tfsdk:"cidr_list"`
	IPSecPSK           types.String   `tfsdk:"ipsec_psk"`
	IKEPolicy          types.String   `tfsdk:"ike_policy"`
	ESPPolicy          types.String   `tfsdk:"esp_policy"`
	IKELifetime        types.String   `tfsdk:"ike_lifetime"`
	ESPLifetime        types.String   `tfsdk:"esp_lifetime"`
	IKEEncryption      types.String   `tfsdk:"ike_encryption"`
	IKEHash            types.String   `tfsdk:"ike_hash"`
	IKEVersion         types.String   `tfsdk:"ike_version"`
	IKEDH              types.String   `tfsdk:"ike_dh"`
	ESPEncryption      types.String   `tfsdk:"esp_encryption"`
	ESPHash            types.String   `tfsdk:"esp_hash"`
	ESPDH              types.String   `tfsdk:"esp_dh"`
	ESPPFS             types.String   `tfsdk:"esp_pfs"`
	ForceEncapsulation types.Bool     `tfsdk:"force_encapsulation"`
	SplitConnections   types.Bool     `tfsdk:"split_connections"`
	DeadPeerDetection  types.Bool     `tfsdk:"dead_peer_detection"`
	CloudProvider      types.String   `tfsdk:"cloud_provider"`
	Region             types.String   `tfsdk:"region"`
	Project            types.String   `tfsdk:"project"`
	Timeouts           timeouts.Value `tfsdk:"timeouts"`
}

func NewVPNCustomerGatewayResource() resource.Resource {
	return &vpnCustomerGatewayResource{}
}

func (r *vpnCustomerGatewayResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vpn_customer_gateway"
}

func (r *vpnCustomerGatewayResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP VPN customer gateway.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Customer gateway slug (unique identifier).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name for the customer gateway.",
			},
			"gateway": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Remote gateway IP address.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"cidr_list": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Comma-separated list of CIDRs reachable behind the remote gateway.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"ipsec_psk": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "IPSec pre-shared key. Write-only — not returned by the API.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"ike_policy": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "IKE policy string (e.g. `aes128-sha1-dh5`).",
			},
			"esp_policy": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ESP policy string (e.g. `aes128-sha1`).",
			},
			"ike_lifetime": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "IKE SA lifetime in seconds (e.g. `86400`).",
			},
			"esp_lifetime": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "ESP SA lifetime in seconds (e.g. `3600`).",
			},
			"ike_encryption": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "IKE encryption algorithm (e.g. `aes128`). Write-only — not returned by the API.",
			},
			"ike_hash": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "IKE hash algorithm (e.g. `sha1`). Write-only — not returned by the API.",
			},
			"ike_version": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "IKE version (`ike`, `ikev1`, or `ikev2`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"ike_dh": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "IKE Diffie-Hellman group (e.g. `modp1024`, `modp2048`). Write-only — not returned by the API.",
			},
			"esp_encryption": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "ESP encryption algorithm (e.g. `aes128`). Write-only — not returned by the API.",
			},
			"esp_hash": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "ESP hash algorithm (e.g. `sha1`). Write-only — not returned by the API.",
			},
			"esp_dh": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "ESP Diffie-Hellman group (e.g. `modp1024`, `modp2048`). Write-only — not returned by the API.",
			},
			"esp_pfs": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "ESP Perfect Forward Secrecy group (e.g. `modp1024`, `modp2048`). Write-only — not returned by the API.",
			},
			"force_encapsulation": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Force UDP encapsulation for NAT traversal.",
			},
			"split_connections": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable split-connection mode.",
			},
			"dead_peer_detection": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable Dead Peer Detection.",
			},
			"cloud_provider": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cloud provider slug (e.g. `nimbo`). Use `data.zcp_region.<name>.cloud_provider` instead of hardcoding.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Region slug where the customer gateway is created (e.g. `yow-1`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project slug. Inherits from the provider `default_project` if omitted.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

func (r *vpnCustomerGatewayResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = vpn.NewCustomerGatewayService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

func (r *vpnCustomerGatewayResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model vpnCustomerGatewayResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vpn_customer_gateway cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	project := r.defaultProject
	if !model.Project.IsNull() && !model.Project.IsUnknown() {
		project = model.Project.ValueString()
	}

	cg, err := r.svc.Create(ctx, vpn.CustomerGatewayRequest{
		Name:               model.Name.ValueString(),
		Gateway:            model.Gateway.ValueString(),
		CIDRList:           model.CIDRList.ValueString(),
		IPSecPSK:           model.IPSecPSK.ValueString(),
		IKEPolicy:          model.IKEPolicy.ValueString(),
		ESPPolicy:          model.ESPPolicy.ValueString(),
		IKELifetime:        model.IKELifetime.ValueString(),
		ESPLifetime:        model.ESPLifetime.ValueString(),
		IKEEncryption:      model.IKEEncryption.ValueString(),
		IKEHash:            model.IKEHash.ValueString(),
		IKEVersion:         model.IKEVersion.ValueString(),
		IKEDH:              model.IKEDH.ValueString(),
		ESPEncryption:      model.ESPEncryption.ValueString(),
		ESPHash:            model.ESPHash.ValueString(),
		ESPDH:              model.ESPDH.ValueString(),
		ESPPFS:             model.ESPPFS.ValueString(),
		ForceEncapsulation: model.ForceEncapsulation.ValueBool(),
		SplitConnections:   model.SplitConnections.ValueBool(),
		DeadPeerDetection:  model.DeadPeerDetection.ValueBool(),
		CloudProvider:      model.CloudProvider.ValueString(),
		Region:             model.Region.ValueString(),
		Project:            project,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create VPN customer gateway", err.Error())
		return
	}

	model.ID = types.StringValue(cg.Slug)
	if cg.Name != "" {
		model.Name = types.StringValue(cg.Name)
	}
	if cg.Gateway != "" {
		model.Gateway = types.StringValue(cg.Gateway)
	}
	if cg.CIDRList != "" {
		model.CIDRList = types.StringValue(cg.CIDRList)
	}
	if cg.IKEPolicy != "" {
		model.IKEPolicy = types.StringValue(cg.IKEPolicy)
	}
	if cg.ESPPolicy != "" {
		model.ESPPolicy = types.StringValue(cg.ESPPolicy)
	}
	if cg.IKELifetime != "" {
		model.IKELifetime = types.StringValue(cg.IKELifetime)
	}
	if cg.ESPLifetime != "" {
		model.ESPLifetime = types.StringValue(cg.ESPLifetime)
	}
	if cg.IKEVersion != "" {
		model.IKEVersion = types.StringValue(cg.IKEVersion)
	}
	// ipsec_psk, ike_encryption, ike_hash, ike_dh, esp_encryption, esp_hash, esp_dh, esp_pfs,
	// cloud_provider, region, and project are write-only — preserved from the plan value already in model.
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *vpnCustomerGatewayResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model vpnCustomerGatewayResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vpn_customer_gateway cannot be read: bearer_token is missing.")
		return
	}

	gateways, err := r.svc.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read VPN customer gateway", err.Error())
		return
	}

	slug := model.ID.ValueString()
	for _, cg := range gateways {
		if cg.Slug == slug {
			model.Name = types.StringValue(cg.Name)
			if cg.Gateway != "" {
				model.Gateway = types.StringValue(cg.Gateway)
			}
			if cg.CIDRList != "" {
				model.CIDRList = types.StringValue(cg.CIDRList)
			}
			if cg.IKEPolicy != "" {
				model.IKEPolicy = types.StringValue(cg.IKEPolicy)
			}
			if cg.ESPPolicy != "" {
				model.ESPPolicy = types.StringValue(cg.ESPPolicy)
			}
			if cg.IKELifetime != "" {
				model.IKELifetime = types.StringValue(cg.IKELifetime)
			}
			if cg.ESPLifetime != "" {
				model.ESPLifetime = types.StringValue(cg.ESPLifetime)
			}
			if cg.IKEVersion != "" {
				model.IKEVersion = types.StringValue(cg.IKEVersion)
			}
			model.ForceEncapsulation = types.BoolValue(cg.ForceEncapsulation)
			model.SplitConnections = types.BoolValue(cg.SplitConnections == "true" || cg.SplitConnections == "1")
			model.DeadPeerDetection = types.BoolValue(cg.DeadPeerDetection)
			// ipsec_psk, ike_encryption, ike_hash, ike_dh, esp_encryption, esp_hash, esp_dh, esp_pfs,
			// cloud_provider, region, and project are write-only — preserved from state.
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *vpnCustomerGatewayResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model vpnCustomerGatewayResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state vpnCustomerGatewayResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vpn_customer_gateway cannot be updated: bearer_token is missing.")
		return
	}

	// Preserve write-only fields from state so they are not blanked in the request.
	project := r.defaultProject
	if !state.Project.IsNull() && !state.Project.IsUnknown() {
		project = state.Project.ValueString()
	}

	_, err := r.svc.Update(ctx, state.ID.ValueString(), vpn.CustomerGatewayRequest{
		Name:               model.Name.ValueString(),
		Gateway:            state.Gateway.ValueString(),
		CIDRList:           state.CIDRList.ValueString(),
		IPSecPSK:           state.IPSecPSK.ValueString(),
		IKEPolicy:          model.IKEPolicy.ValueString(),
		ESPPolicy:          model.ESPPolicy.ValueString(),
		IKELifetime:        model.IKELifetime.ValueString(),
		ESPLifetime:        model.ESPLifetime.ValueString(),
		IKEEncryption:      state.IKEEncryption.ValueString(),
		IKEHash:            state.IKEHash.ValueString(),
		IKEVersion:         state.IKEVersion.ValueString(),
		IKEDH:              state.IKEDH.ValueString(),
		ESPEncryption:      state.ESPEncryption.ValueString(),
		ESPHash:            state.ESPHash.ValueString(),
		ESPDH:              state.ESPDH.ValueString(),
		ESPPFS:             state.ESPPFS.ValueString(),
		ForceEncapsulation: model.ForceEncapsulation.ValueBool(),
		SplitConnections:   model.SplitConnections.ValueBool(),
		DeadPeerDetection:  model.DeadPeerDetection.ValueBool(),
		CloudProvider:      state.CloudProvider.ValueString(),
		Region:             state.Region.ValueString(),
		Project:            project,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to update VPN customer gateway", err.Error())
		return
	}

	// Merge: plan has the new mutable fields; preserve immutable and write-only from state.
	model.ID = state.ID
	model.Gateway = state.Gateway
	model.CIDRList = state.CIDRList
	model.IPSecPSK = state.IPSecPSK
	model.IKEEncryption = state.IKEEncryption
	model.IKEHash = state.IKEHash
	model.IKEVersion = state.IKEVersion
	model.IKEDH = state.IKEDH
	model.ESPEncryption = state.ESPEncryption
	model.ESPHash = state.ESPHash
	model.ESPDH = state.ESPDH
	model.ESPPFS = state.ESPPFS
	model.CloudProvider = state.CloudProvider
	model.Region = state.Region
	model.Project = state.Project
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *vpnCustomerGatewayResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model vpnCustomerGatewayResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vpn_customer_gateway cannot be deleted: bearer_token is missing.")
		return
	}

	deleteTimeout, diags := model.Timeouts.Delete(ctx, 2*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	slug := model.ID.ValueString()
	err := r.svc.Delete(ctx, slug)
	if err != nil && !apierrors.IsNotFound(err) && !apierrors.IsResourceNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete VPN customer gateway", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		_, err := r.svc.Get(ctx, slug)
		if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
			return false, nil
		}
		return err == nil, err
	}); err != nil {
		resp.Diagnostics.AddError("VPN customer gateway deletion did not complete", err.Error())
	}
}

func (r *vpnCustomerGatewayResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
