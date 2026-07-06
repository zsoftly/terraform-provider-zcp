package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/storagecategory"
)

var _ datasource.DataSource = &storageCategoryDataSource{}

type storageCategoryLister interface {
	List(ctx context.Context, regionSlug string) ([]storagecategory.StorageCategory, error)
}

type storageCategoryDataSource struct {
	svc storageCategoryLister
}

type storageCategoryDataSourceModel struct {
	Slug   types.String `tfsdk:"slug"`
	Region types.String `tfsdk:"region"`
	ID     types.String `tfsdk:"id"`
	Name   types.String `tfsdk:"name"`
	Status types.Bool   `tfsdk:"status"`
}

func NewStorageCategoryDataSource() datasource.DataSource {
	return &storageCategoryDataSource{}
}

func (d *storageCategoryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storage_category"
}

func (d *storageCategoryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a ZCP storage category by slug. Categories are region-specific (e.g. `nvme` in YOW, `pro-nvme` in YUL), so pass `region` to scope the lookup.",
		Attributes: map[string]schema.Attribute{
			"slug": schema.StringAttribute{
				MarkdownDescription: "Storage category slug (e.g. `nvme`, `pro-nvme`).",
				Required:            true,
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "Region slug to scope the lookup (e.g. `yow-1`).",
				Optional:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Storage category ID.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Storage category display name.",
				Computed:            true,
			},
			"status": schema.BoolAttribute{
				MarkdownDescription: "Whether the storage category is enabled.",
				Computed:            true,
			},
		},
	}
}

func (d *storageCategoryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	d.svc = storagecategory.NewService(pd.Client)
}

func (d *storageCategoryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state storageCategoryDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_storage_category cannot be read: the provider was not configured successfully. Ensure bearer_token is set.")
		return
	}

	region := ""
	if !state.Region.IsNull() && !state.Region.IsUnknown() {
		region = state.Region.ValueString()
	}

	categories, err := d.svc.List(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list storage categories", err.Error())
		return
	}

	slug := state.Slug.ValueString()
	for _, c := range categories {
		if c.Slug == slug {
			state.ID = types.StringValue(c.ID)
			state.Name = types.StringValue(c.Name)
			state.Status = types.BoolValue(c.Status)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.Diagnostics.AddError(
		"Storage category not found",
		fmt.Sprintf("No storage category with slug %q exists%s.", slug, regionScopeSuffix(region)),
	)
}

// regionScopeSuffix renders " in region <slug>" for lookup error messages.
func regionScopeSuffix(region string) string {
	if region == "" {
		return ""
	}
	return fmt.Sprintf(" in region %q", region)
}
