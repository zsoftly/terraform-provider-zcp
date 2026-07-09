package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/network"
)

var _ datasource.DataSource = &networkDataSource{}

type networkLister interface {
	List(ctx context.Context, region, project string) ([]network.Network, error)
}

type networkDataSource struct {
	svc            networkLister
	defaultProject string
}

type networkDataSourceModel struct {
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

func NewNetworkDataSource() datasource.DataSource {
	return &networkDataSource{}
}

func (d *networkDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}

func (d *networkDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing ZCP network by slug, for example to place instances into a network created outside Terraform.",
		Attributes: map[string]schema.Attribute{
			"slug": schema.StringAttribute{
				MarkdownDescription: "Network slug.",
				Required:            true,
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "Region slug to scope the lookup (e.g. `yow-1`).",
				Optional:            true,
			},
			"project": schema.StringAttribute{
				MarkdownDescription: "Project slug to scope the lookup. Inherits from the provider `default_project` if omitted.",
				Optional:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Network slug (same as `slug`).",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Network display name.",
				Computed:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Network type (e.g. `Isolated`, `L2`).",
				Computed:            true,
			},
			"gateway": schema.StringAttribute{
				MarkdownDescription: "Gateway address.",
				Computed:            true,
			},
			"cidr": schema.StringAttribute{
				MarkdownDescription: "Network CIDR.",
				Computed:            true,
			},
			"netmask": schema.StringAttribute{
				MarkdownDescription: "Netmask.",
				Computed:            true,
			},
			"category": schema.StringAttribute{
				MarkdownDescription: "Network category.",
				Computed:            true,
			},
			"vpc": schema.StringAttribute{
				MarkdownDescription: "Parent VPC slug when the network is a VPC tier.",
				Computed:            true,
			},
			"is_default": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the default network.",
				Computed:            true,
			},
			"zone_name": schema.StringAttribute{
				MarkdownDescription: "Zone name.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Network description.",
				Computed:            true,
			},
		},
	}
}

func (d *networkDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	d.svc = network.NewService(pd.Client)
	d.defaultProject = pd.DefaultProject
}

func (d *networkDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state networkDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_network cannot be read: the provider was not configured successfully. Ensure bearer_token is set.")
		return
	}

	region := ""
	if !state.Region.IsNull() && !state.Region.IsUnknown() {
		region = state.Region.ValueString()
	}
	project := d.defaultProject
	if !state.Project.IsNull() && !state.Project.IsUnknown() {
		project = state.Project.ValueString()
	}

	networks, err := d.svc.List(ctx, region, project)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list networks", err.Error())
		return
	}

	slug := state.Slug.ValueString()
	for _, n := range networks {
		if n.Slug == slug {
			state.ID = types.StringValue(n.Slug)
			state.Name = types.StringValue(n.Name)
			state.Type = types.StringValue(n.NetworkType)
			state.Gateway = types.StringValue(n.Gateway)
			state.CIDR = types.StringValue(n.CIDR)
			state.Netmask = types.StringValue(n.Netmask)
			state.Category = types.StringValue(n.Category)
			state.VPC = types.StringValue(n.VPC)
			state.IsDefault = types.BoolValue(n.IsDefault)
			state.ZoneName = types.StringValue(n.ZoneName)
			state.Description = types.StringValue(n.Description)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.Diagnostics.AddError(
		"Network not found",
		fmt.Sprintf("No network with slug %q exists%s.", slug, regionScopeSuffix(region)),
	)
}
