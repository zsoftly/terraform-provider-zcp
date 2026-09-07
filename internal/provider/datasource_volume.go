package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/volume"
)

var _ datasource.DataSource = &volumeDataSource{}

type volumeDataSource struct {
	svc            volumeLister
	defaultProject string
}

type volumeDataSourceModel struct {
	Slug       types.String `tfsdk:"slug"`
	Region     types.String `tfsdk:"region"`
	Project    types.String `tfsdk:"project"`
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Size       types.Int64  `tfsdk:"size"`
	VolumeType types.String `tfsdk:"volume_type"`
	InstanceID types.String `tfsdk:"instance_id"`
	CreatedAt  types.String `tfsdk:"created_at"`
}

func NewVolumeDataSource() datasource.DataSource {
	return &volumeDataSource{}
}

func (d *volumeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_volume"
}

func (d *volumeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing ZCP block storage volume by slug, for example to find the slug of an instance's root volume for `zcp_volume_backup` without hardcoding it.",
		Attributes: map[string]schema.Attribute{
			"slug": schema.StringAttribute{
				MarkdownDescription: "Volume slug.",
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
				MarkdownDescription: "Volume slug (same as `slug`).",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Volume display name.",
				Computed:            true,
			},
			"size": schema.Int64Attribute{
				MarkdownDescription: "Storage size in GB.",
				Computed:            true,
			},
			"volume_type": schema.StringAttribute{
				MarkdownDescription: "Volume type (`ROOT` for a boot disk, empty for a data volume).",
				Computed:            true,
			},
			"instance_id": schema.StringAttribute{
				MarkdownDescription: "ID of the virtual machine this volume is attached to, empty when detached.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Timestamp the volume was created.",
				Computed:            true,
			},
		},
	}
}

func (d *volumeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	d.svc = volume.NewService(pd.Client)
	d.defaultProject = pd.DefaultProject
}

func (d *volumeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state volumeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_volume cannot be read: the provider was not configured successfully. Ensure bearer_token is set.")
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

	// volume.Service.List fetches a single page; the SDK has no page parameter
	// yet. On an account with more volumes than fit on one API page, scope the
	// lookup with region and project to keep the target volume on that page.
	volumes, err := d.svc.List(ctx, region, project)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list volumes", err.Error())
		return
	}

	slug := state.Slug.ValueString()
	for _, v := range volumes {
		if v.Slug != slug {
			continue
		}
		size, err := v.Size.Int64()
		if err != nil {
			resp.Diagnostics.AddError(
				"Invalid volume size",
				fmt.Sprintf("Volume %q returned a size that is not an integer: %q.", v.Slug, v.Size.String()),
			)
			return
		}
		state.ID = types.StringValue(v.Slug)
		state.Name = types.StringValue(v.Name)
		state.Size = types.Int64Value(size)
		state.VolumeType = types.StringValue(v.VolumeType)
		state.InstanceID = types.StringValue(v.VirtualMachineID)
		state.CreatedAt = types.StringValue(v.CreatedAt)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	resp.Diagnostics.AddError(
		"Volume not found",
		fmt.Sprintf("No volume with slug %q exists%s.", slug, regionScopeSuffix(region)),
	)
}
