package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/instance"
	"github.com/zsoftly/zcp-cli/pkg/api/volume"
)

var _ datasource.DataSource = &instanceDataSource{}

type instanceGetter interface {
	Get(ctx context.Context, slug string) (*instance.VirtualMachine, error)
}

type volumeLister interface {
	List(ctx context.Context, region, project string) ([]volume.Volume, error)
}

type instanceDataSource struct {
	svc            instanceGetter
	volSvc         volumeLister
	defaultProject string
}

type instanceDataSourceModel struct {
	Slug       types.String `tfsdk:"slug"`
	Region     types.String `tfsdk:"region"`
	Project    types.String `tfsdk:"project"`
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	State      types.String `tfsdk:"state"`
	PrivateIP  types.String `tfsdk:"private_ip"`
	PublicIP   types.String `tfsdk:"public_ip"`
	RootVolume types.String `tfsdk:"root_volume"`
	Volumes    types.List   `tfsdk:"volumes"`
}

func NewInstanceDataSource() datasource.DataSource {
	return &instanceDataSource{}
}

func (d *instanceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_instance"
}

func (d *instanceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing ZCP instance by slug, for example to attach resources to an instance created outside Terraform.",
		Attributes: map[string]schema.Attribute{
			"slug": schema.StringAttribute{
				MarkdownDescription: "Instance slug.",
				Required:            true,
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "Region slug to scope the attached-volume lookup (e.g. `yow-1`). If omitted, volumes are listed across all regions and filtered to this instance.",
				Optional:            true,
			},
			"project": schema.StringAttribute{
				MarkdownDescription: "Project slug to scope the attached-volume lookup. Inherits from the provider `default_project` if omitted.",
				Optional:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Instance slug (same as `slug`).",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Instance display name.",
				Computed:            true,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: "Current power state.",
				Computed:            true,
			},
			"private_ip": schema.StringAttribute{
				MarkdownDescription: "Private IP address.",
				Computed:            true,
			},
			"public_ip": schema.StringAttribute{
				MarkdownDescription: "Public IP address, if assigned.",
				Computed:            true,
			},
			"root_volume": schema.StringAttribute{
				MarkdownDescription: "Slug of the root volume attached to this instance. Empty if no root volume is found. Use this to point `zcp_volume_backup` at the instance's boot disk without hardcoding a slug that changes when the instance is rebuilt.",
				Computed:            true,
			},
			"volumes": schema.ListAttribute{
				MarkdownDescription: "Slugs of all volumes attached to this instance, root volume first.",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

func (d *instanceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	d.svc = instance.NewService(pd.Client)
	d.volSvc = volume.NewService(pd.Client)
	d.defaultProject = pd.DefaultProject
}

func (d *instanceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state instanceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_instance cannot be read: the provider was not configured successfully. Ensure bearer_token is set.")
		return
	}

	vm, err := d.svc.Get(ctx, state.Slug.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Instance not found", fmt.Sprintf("Cannot look up instance %q: %s", state.Slug.ValueString(), err))
		return
	}

	state.ID = types.StringValue(vm.Slug)
	state.Name = types.StringValue(vm.Name)
	state.State = types.StringValue(vm.State)
	state.PrivateIP = types.StringValue(vm.NetworkPrivateIP())
	if publicIP := vm.GetPublicIPAddress(); publicIP != "" {
		state.PublicIP = types.StringValue(publicIP)
	} else {
		state.PublicIP = types.StringValue(instance.StringVal(vm.PublicIP))
	}

	region := ""
	if !state.Region.IsNull() && !state.Region.IsUnknown() {
		region = state.Region.ValueString()
	}
	project := d.defaultProject
	if !state.Project.IsNull() && !state.Project.IsUnknown() {
		project = state.Project.ValueString()
	}

	// A data source configured only with an instanceGetter (no volume lister)
	// still returns the instance fields. The volume attributes simply come
	// back empty rather than failing the read. A volume-listing failure (e.g. a
	// token without block-storage read permission) is likewise not fatal: it
	// only warns and leaves root_volume/volumes empty, since the instance
	// fields the caller most likely wants are already resolved.
	rootVolume := ""
	volumeSlugs := []string{}
	if d.volSvc != nil {
		rootVolume, volumeSlugs, err = d.attachedVolumes(ctx, region, project, vm.ID)
		if err != nil {
			resp.Diagnostics.AddWarning(
				"Failed to list volumes",
				fmt.Sprintf("root_volume and volumes will be empty for instance %q: %s", state.Slug.ValueString(), err),
			)
			rootVolume = ""
			volumeSlugs = []string{}
		}
	}
	state.RootVolume = types.StringValue(rootVolume)
	volumesList, diags := types.ListValueFrom(ctx, types.StringType, volumeSlugs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Volumes = volumesList

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// attachedVolumes lists volumes scoped to region/project and returns the slug
// of the ROOT volume attached to vmID (empty if none) plus the slugs of all
// attached volumes, root first.
//
// The released zcp-cli v0.0.29 volume.Service.List retrieves every result page
// within the requested region and project scope.
func (d *instanceDataSource) attachedVolumes(ctx context.Context, region, project, vmID string) (string, []string, error) {
	volumes, err := d.volSvc.List(ctx, region, project)
	if err != nil {
		return "", nil, err
	}

	var rootSlug string
	extraRoots := []string{}
	dataSlugs := []string{}
	for _, v := range volumes {
		if v.VirtualMachineID != vmID {
			continue
		}
		if v.VolumeType == "ROOT" {
			if rootSlug == "" {
				rootSlug = v.Slug
			} else {
				// A second ROOT volume on the same VM: keep it right after the
				// first root, ahead of ordinary data disks.
				extraRoots = append(extraRoots, v.Slug)
			}
			continue
		}
		dataSlugs = append(dataSlugs, v.Slug)
	}
	slugs := append(extraRoots, dataSlugs...)
	if rootSlug != "" {
		slugs = append([]string{rootSlug}, slugs...)
	}
	return rootSlug, slugs, nil
}
