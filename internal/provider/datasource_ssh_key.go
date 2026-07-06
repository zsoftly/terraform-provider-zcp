package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/sshkey"
)

var _ datasource.DataSource = &sshKeyDataSource{}

type sshKeyLister interface {
	List(ctx context.Context) ([]sshkey.SSHKey, error)
}

type sshKeyDataSource struct {
	svc sshKeyLister
}

type sshKeyDataSourceModel struct {
	Slug      types.String `tfsdk:"slug"`
	Name      types.String `tfsdk:"name"`
	ID        types.String `tfsdk:"id"`
	PublicKey types.String `tfsdk:"public_key"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func NewSSHKeyDataSource() datasource.DataSource {
	return &sshKeyDataSource{}
}

func (d *sshKeyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssh_key"
}

func (d *sshKeyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing ZCP SSH key by slug or name, for example to reference a key uploaded through the console.",
		Attributes: map[string]schema.Attribute{
			"slug": schema.StringAttribute{
				MarkdownDescription: "SSH key slug. Exactly one of `slug` or `name` must be set.",
				Optional:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "SSH key display name. Exactly one of `slug` or `name` must be set.",
				Optional:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "SSH key slug.",
				Computed:            true,
			},
			"public_key": schema.StringAttribute{
				MarkdownDescription: "OpenSSH public key material.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp.",
				Computed:            true,
			},
		},
	}
}

func (d *sshKeyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	d.svc = sshkey.NewService(pd.Client)
}

func (d *sshKeyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state sshKeyDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_ssh_key cannot be read: the provider was not configured successfully. Ensure bearer_token is set.")
		return
	}

	slugSet := !state.Slug.IsNull() && !state.Slug.IsUnknown()
	nameSet := !state.Name.IsNull() && !state.Name.IsUnknown()
	if slugSet == nameSet {
		resp.Diagnostics.AddError(
			"Invalid SSH key lookup",
			"Set exactly one of `slug` or `name` to look up an SSH key.",
		)
		return
	}

	keys, err := d.svc.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list SSH keys", err.Error())
		return
	}

	for _, k := range keys {
		if (slugSet && k.Slug == state.Slug.ValueString()) || (nameSet && k.Name == state.Name.ValueString()) {
			state.ID = types.StringValue(k.Slug)
			state.Slug = types.StringValue(k.Slug)
			state.Name = types.StringValue(k.Name)
			state.PublicKey = types.StringValue(k.PublicKey)
			state.CreatedAt = types.StringValue(k.CreatedAt)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

	lookup := state.Name.ValueString()
	if slugSet {
		lookup = state.Slug.ValueString()
	}
	resp.Diagnostics.AddError(
		"SSH key not found",
		fmt.Sprintf("No SSH key matching %q exists.", lookup),
	)
}
