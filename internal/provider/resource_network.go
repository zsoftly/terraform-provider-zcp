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
	"github.com/zsoftly/zcp-cli/pkg/api/network"
)

var _ resource.Resource = &networkResource{}
var _ resource.ResourceWithConfigure = &networkResource{}
var _ resource.ResourceWithImportState = &networkResource{}

type networkServiceIface interface {
	List(ctx context.Context) ([]network.Network, error)
	Create(ctx context.Context, req network.CreateRequest) (*network.Network, error)
	Update(ctx context.Context, slug string, req network.UpdateRequest) (*network.Network, error)
	Delete(ctx context.Context, slug string) error
}

type networkResource struct {
	svc            networkServiceIface
	defaultProject string
}

type networkResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	CloudProvider types.String `tfsdk:"cloud_provider"`
	Region        types.String `tfsdk:"region"`
	Project       types.String `tfsdk:"project"`
	Description   types.String `tfsdk:"description"`
	CategorySlug  types.String `tfsdk:"category_slug"`
	// Computed
	Gateway  types.String   `tfsdk:"gateway"`
	CIDR     types.String   `tfsdk:"cidr"`
	Netmask  types.String   `tfsdk:"netmask"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func NewNetworkResource() resource.Resource {
	return &networkResource{}
}

func (r *networkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}

func (r *networkResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP network.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Network slug (unique identifier).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name for the network.",
			},
			"cloud_provider": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cloud provider slug (e.g. `nimbo`). Use `data.zcp_region.<name>.cloud_provider` instead of hardcoding.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Region slug where the network is created.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project slug. Inherits from the provider `default_project` if omitted.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Human-readable description.",
			},
			"category_slug": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Network category slug. Not returned by the API after creation; changes force replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"gateway": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Network gateway IP address.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cidr": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Network CIDR block.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"netmask": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Network subnet mask.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
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

func (r *networkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = network.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

func (r *networkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model networkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_network cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 10*time.Minute)
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

	net, err := r.svc.Create(ctx, network.CreateRequest{
		Name:          model.Name.ValueString(),
		CloudProvider: model.CloudProvider.ValueString(),
		Region:        model.Region.ValueString(),
		Project:       project,
		Description:   model.Description.ValueString(),
		CategorySlug:  model.CategorySlug.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create network", err.Error())
		return
	}

	model.ID = types.StringValue(net.Slug)
	model.Gateway = types.StringValue(net.Gateway)
	model.CIDR = types.StringValue(net.CIDR)
	model.Netmask = types.StringValue(net.Netmask)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *networkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model networkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_network cannot be read: bearer_token is missing.")
		return
	}

	networks, err := r.svc.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read network", err.Error())
		return
	}

	slug := model.ID.ValueString()
	for _, n := range networks {
		if n.Slug == slug {
			model.Name = types.StringValue(n.Name)
			model.Description = types.StringValue(n.Description)
			model.Gateway = types.StringValue(n.Gateway)
			model.CIDR = types.StringValue(n.CIDR)
			model.Netmask = types.StringValue(n.Netmask)
			// cloud_provider, region, project, category_slug are write-only (not in API response);
			// preserved from state.
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *networkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model networkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state networkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_network cannot be updated: bearer_token is missing.")
		return
	}

	desc := model.Description.ValueString()
	_, err := r.svc.Update(ctx, state.ID.ValueString(), network.UpdateRequest{
		Name:        model.Name.ValueString(),
		Description: &desc,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to update network", err.Error())
		return
	}

	// Merge: plan has the new name/description; preserve computed and write-only from state.
	model.ID = state.ID
	model.Gateway = state.Gateway
	model.CIDR = state.CIDR
	model.Netmask = state.Netmask
	model.CategorySlug = state.CategorySlug
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *networkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model networkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_network cannot be deleted: bearer_token is missing.")
		return
	}
	deleteTimeout, diags := model.Timeouts.Delete(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	slug := model.ID.ValueString()
	err := r.svc.Delete(ctx, slug)
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete network", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		networks, err := r.svc.List(ctx)
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for _, n := range networks {
			if n.Slug == slug {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError("Network deletion did not complete", err.Error())
	}
}

func (r *networkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
