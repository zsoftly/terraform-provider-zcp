package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/permission"
)

var _ datasource.DataSource = &permissionsDataSource{}

type permissionLister interface {
	List(ctx context.Context) ([]permission.Permission, error)
}

type permissionsDataSource struct {
	svc permissionLister
}

type permissionModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Slug        types.String `tfsdk:"slug"`
	Description types.String `tfsdk:"description"`
	Category    types.String `tfsdk:"category"`
}

type permissionsDataSourceModel struct {
	Category    types.String      `tfsdk:"category"`
	ID          types.String      `tfsdk:"id"`
	Permissions []permissionModel `tfsdk:"permissions"`
}

func NewPermissionsDataSource() datasource.DataSource {
	return &permissionsDataSource{}
}

func (d *permissionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permissions"
}

func (d *permissionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the permission catalog for building `zcp_role` resources, optionally filtered by category.",
		Attributes: map[string]schema.Attribute{
			"category": schema.StringAttribute{
				MarkdownDescription: "Return permissions in this category only.",
				Optional:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Synthetic identifier.",
				Computed:            true,
			},
			"permissions": schema.ListNestedAttribute{
				MarkdownDescription: "Matching permissions.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Permission ID.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Permission display name.",
							Computed:            true,
						},
						"slug": schema.StringAttribute{
							MarkdownDescription: "Permission slug, usable in `zcp_role.permissions`.",
							Computed:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: "Permission description.",
							Computed:            true,
						},
						"category": schema.StringAttribute{
							MarkdownDescription: "Permission category.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *permissionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	d.svc = permission.NewService(pd.Client)
}

func (d *permissionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state permissionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_permissions cannot be read: the provider was not configured successfully. Ensure bearer_token is set.")
		return
	}

	permissions, err := d.svc.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list permissions", err.Error())
		return
	}

	category := ""
	if !state.Category.IsNull() && !state.Category.IsUnknown() {
		category = state.Category.ValueString()
	}

	state.Permissions = make([]permissionModel, 0, len(permissions))
	for _, p := range permissions {
		if category != "" && p.Category != category {
			continue
		}
		state.Permissions = append(state.Permissions, permissionModel{
			ID:          types.StringValue(p.ID),
			Name:        types.StringValue(p.Name),
			Slug:        types.StringValue(p.Slug),
			Description: types.StringValue(p.Description),
			Category:    types.StringValue(p.Category),
		})
	}

	id := "permissions"
	if category != "" {
		id = "permissions-" + category
	}
	state.ID = types.StringValue(id)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
