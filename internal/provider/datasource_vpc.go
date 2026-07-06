package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/vpc"
)

var _ datasource.DataSource = &vpcDataSource{}

type vpcLister interface {
	List(ctx context.Context, zoneSlug, region, project string) ([]vpc.VPC, error)
}

type vpcDataSource struct {
	svc            vpcLister
	defaultProject string
}

type vpcDataSourceModel struct {
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

func NewVPCDataSource() datasource.DataSource {
	return &vpcDataSource{}
}

func (d *vpcDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vpc"
}

func (d *vpcDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing ZCP VPC by slug, for example to add tiers to a VPC created outside Terraform.",
		Attributes: map[string]schema.Attribute{
			"slug": schema.StringAttribute{
				MarkdownDescription: "VPC slug.",
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
				MarkdownDescription: "VPC slug (same as `slug`).",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "VPC display name.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "VPC description.",
				Computed:            true,
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Current VPC status.",
				Computed:            true,
			},
			"cidr": schema.StringAttribute{
				MarkdownDescription: "VPC CIDR.",
				Computed:            true,
			},
			"zone_name": schema.StringAttribute{
				MarkdownDescription: "Zone name.",
				Computed:            true,
			},
			"domain_name": schema.StringAttribute{
				MarkdownDescription: "Network domain name.",
				Computed:            true,
			},
		},
	}
}

func (d *vpcDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	d.svc = vpc.NewService(pd.Client)
	d.defaultProject = pd.DefaultProject
}

func (d *vpcDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state vpcDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vpc cannot be read: the provider was not configured successfully. Ensure bearer_token is set.")
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

	vpcs, err := d.svc.List(ctx, "", region, project)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list VPCs", err.Error())
		return
	}

	slug := state.Slug.ValueString()
	for _, v := range vpcs {
		if v.Slug == slug {
			state.ID = types.StringValue(v.Slug)
			state.Name = types.StringValue(v.Name)
			state.Description = types.StringValue(v.Description)
			state.Status = types.StringValue(v.Status)
			state.CIDR = types.StringValue(v.CIDR)
			state.ZoneName = types.StringValue(v.ZoneName)
			state.DomainName = types.StringValue(v.DomainName)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.Diagnostics.AddError(
		"VPC not found",
		fmt.Sprintf("No VPC with slug %q exists%s.", slug, regionScopeSuffix(region)),
	)
}
