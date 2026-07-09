package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/instance"
)

var _ datasource.DataSource = &instanceDataSource{}

type instanceGetter interface {
	Get(ctx context.Context, slug string) (*instance.VirtualMachine, error)
}

type instanceDataSource struct {
	svc instanceGetter
}

type instanceDataSourceModel struct {
	Slug      types.String `tfsdk:"slug"`
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	State     types.String `tfsdk:"state"`
	PrivateIP types.String `tfsdk:"private_ip"`
	PublicIP  types.String `tfsdk:"public_ip"`
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
	state.PublicIP = types.StringValue(instance.StringVal(vm.PublicIP))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
