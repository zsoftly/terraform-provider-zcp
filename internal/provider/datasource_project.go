package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/project"
)

var _ datasource.DataSource = &projectDataSource{}

// projectLister is satisfied by *project.Service.
type projectLister interface {
	List(ctx context.Context) ([]project.Project, error)
}

type projectDataSource struct {
	svc projectLister
}

type projectDataSourceModel struct {
	Slug        types.String `tfsdk:"slug"`
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func NewProjectDataSource() datasource.DataSource {
	return &projectDataSource{}
}

func (d *projectDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (d *projectDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a ZCP project by slug.",
		Attributes: map[string]schema.Attribute{
			"slug": schema.StringAttribute{
				MarkdownDescription: "Unique project slug.",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Project ID.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Project display name.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Project description.",
				Computed:            true,
			},
		},
	}
}

func (d *projectDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
	d.svc = project.NewService(pd.Client)
}

func (d *projectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state projectDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.svc == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"zcp_project cannot be read: the provider was not configured successfully. Ensure bearer_token is set.",
		)
		return
	}

	projects, err := d.svc.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list projects", err.Error())
		return
	}

	slug := state.Slug.ValueString()
	for _, p := range projects {
		if p.Slug == slug {
			state.ID = types.StringValue(p.ID)
			state.Name = types.StringValue(p.Name)
			state.Description = types.StringValue(p.Description)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Project not found",
		fmt.Sprintf("No project with slug %q exists.", slug),
	)
}
