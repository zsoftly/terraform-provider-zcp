package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/region"
)

var _ datasource.DataSource = &regionDataSource{}

type regionLister interface {
	List(ctx context.Context) ([]region.Region, error)
}

type regionDataSource struct {
	svc regionLister
}

type regionDataSourceModel struct {
	Slug        types.String `tfsdk:"slug"`
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Country     types.String `tfsdk:"country"`
	CountryCode types.String `tfsdk:"country_code"`
}

func NewRegionDataSource() datasource.DataSource {
	return &regionDataSource{}
}

func (d *regionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_region"
}

func (d *regionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a ZCP region by slug.",
		Attributes: map[string]schema.Attribute{
			"slug": schema.StringAttribute{
				MarkdownDescription: "Unique region slug (e.g. `yow-1`).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Region ID.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Region display name.",
				Computed:            true,
			},
			"country": schema.StringAttribute{
				MarkdownDescription: "Full country name.",
				Computed:            true,
			},
			"country_code": schema.StringAttribute{
				MarkdownDescription: "ISO country code.",
				Computed:            true,
			},
		},
	}
}

func (d *regionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData),
		)
		return
	}
	d.svc = region.NewService(pd.Client)
}

func (d *regionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state regionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.svc == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"zcp_region cannot be read: the provider was not configured successfully. Ensure bearer_token is set.",
		)
		return
	}

	regions, err := d.svc.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list regions", err.Error())
		return
	}

	slug := state.Slug.ValueString()
	for _, r := range regions {
		if r.Slug == slug {
			state.ID = types.StringValue(r.ID)
			state.Name = types.StringValue(r.Name)
			state.Country = types.StringValue(r.Country)
			state.CountryCode = types.StringValue(r.CountryCode)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Region not found",
		fmt.Sprintf("No region with slug %q exists.", slug),
	)
}
