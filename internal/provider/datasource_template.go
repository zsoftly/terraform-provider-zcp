package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/template"
)

var _ datasource.DataSource = &templateDataSource{}

type templateLister interface {
	List(ctx context.Context, regionSlug string) ([]template.Template, error)
}

type templateDataSource struct {
	svc templateLister
}

type templateDataSourceModel struct {
	Slug       types.String `tfsdk:"slug"`
	RegionSlug types.String `tfsdk:"region_slug"`
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	RegionID   types.String `tfsdk:"region_id"`
	Type       types.String `tfsdk:"type"`
}

func NewTemplateDataSource() datasource.DataSource {
	return &templateDataSource{}
}

func (d *templateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_template"
}

func (d *templateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a ZCP public VM template by slug.",
		Attributes: map[string]schema.Attribute{
			"slug": schema.StringAttribute{
				MarkdownDescription: "Unique template slug (e.g. `ubuntu-2204-lts`).",
				Required:            true,
			},
			"region_slug": schema.StringAttribute{
				MarkdownDescription: "Narrow the search to a specific region slug. Recommended to avoid ambiguity when the same template exists in multiple regions.",
				Optional:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Template ID.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Template display name.",
				Computed:            true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "ID of the region this template belongs to.",
				Computed:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Template type (e.g. `Template`).",
				Computed:            true,
			},
		},
	}
}

func (d *templateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
	d.svc = template.NewService(pd.Client)
}

func (d *templateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state templateDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.svc == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"zcp_template cannot be read: the provider was not configured successfully. Ensure bearer_token is set.",
		)
		return
	}

	regionSlug := ""
	if !state.RegionSlug.IsNull() && !state.RegionSlug.IsUnknown() {
		regionSlug = state.RegionSlug.ValueString()
	}

	templates, err := d.svc.List(ctx, regionSlug)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list templates", err.Error())
		return
	}

	slug := state.Slug.ValueString()
	for _, t := range templates {
		if t.Slug == slug {
			state.ID = types.StringValue(t.ID)
			state.Name = types.StringValue(t.Name)
			state.RegionID = types.StringValue(t.RegionID)
			state.Type = types.StringValue(t.Type)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Template not found",
		fmt.Sprintf("No template with slug %q exists%s.", slug, regionQualifier(regionSlug)),
	)
}

func regionQualifier(slug string) string {
	if slug == "" {
		return ""
	}
	return " in region " + slug
}
