package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/kubernetes"
)

var _ datasource.DataSource = &kubernetesVersionDataSource{}

type kubernetesVersionLister interface {
	ListVersions(ctx context.Context) ([]kubernetes.KubernetesVersion, error)
}

type kubernetesVersionDataSource struct {
	svc kubernetesVersionLister
}

type kubernetesVersionDataSourceModel struct {
	Slug             types.String `tfsdk:"slug"`
	Version          types.String `tfsdk:"version"`
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	ClusterVersionID types.String `tfsdk:"cluster_version_id"`
	RegionID         types.String `tfsdk:"region_id"`
}

func NewKubernetesVersionDataSource() datasource.DataSource {
	return &kubernetesVersionDataSource{}
}

func (d *kubernetesVersionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kubernetes_version"
}

func (d *kubernetesVersionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an available ZCP Kubernetes version by slug or version string, for use with `zcp_kubernetes_cluster`.",
		Attributes: map[string]schema.Attribute{
			"slug": schema.StringAttribute{
				MarkdownDescription: "Version slug. Exactly one of `slug` or `version` must be set.",
				Optional:            true,
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "Version string (e.g. `1.32.0`). Exactly one of `slug` or `version` must be set.",
				Optional:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Version ID.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Version display name.",
				Computed:            true,
			},
			"cluster_version_id": schema.StringAttribute{
				MarkdownDescription: "Kubernetes cluster version ID.",
				Computed:            true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "Region ID the version is available in.",
				Computed:            true,
			},
		},
	}
}

func (d *kubernetesVersionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	d.svc = kubernetes.NewService(pd.Client)
}

func (d *kubernetesVersionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state kubernetesVersionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_kubernetes_version cannot be read: the provider was not configured successfully. Ensure bearer_token is set.")
		return
	}

	slugSet := !state.Slug.IsNull() && !state.Slug.IsUnknown()
	versionSet := !state.Version.IsNull() && !state.Version.IsUnknown()
	if slugSet == versionSet {
		resp.Diagnostics.AddError(
			"Invalid Kubernetes version lookup",
			"Set exactly one of `slug` or `version` to look up a Kubernetes version.",
		)
		return
	}

	versions, err := d.svc.ListVersions(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list Kubernetes versions", err.Error())
		return
	}

	for _, v := range versions {
		if (slugSet && v.Slug == state.Slug.ValueString()) || (versionSet && v.Version == state.Version.ValueString()) {
			state.ID = types.StringValue(v.ID)
			state.Slug = types.StringValue(v.Slug)
			state.Version = types.StringValue(v.Version)
			state.Name = types.StringValue(v.Name)
			state.ClusterVersionID = types.StringValue(v.KubernetesClusterVersionID)
			state.RegionID = types.StringValue(v.RegionID)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

	lookup := state.Version.ValueString()
	if slugSet {
		lookup = state.Slug.ValueString()
	}
	resp.Diagnostics.AddError(
		"Kubernetes version not found",
		fmt.Sprintf("No Kubernetes version matching %q exists.", lookup),
	)
}
