package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/billingcycle"
)

var _ datasource.DataSource = &billingCycleDataSource{}

type billingCycleLister interface {
	List(ctx context.Context) ([]billingcycle.BillingCycle, error)
}

type billingCycleDataSource struct {
	svc billingCycleLister
}

type billingCycleDataSourceModel struct {
	Slug        types.String `tfsdk:"slug"`
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Duration    types.Int64  `tfsdk:"duration"`
	Unit        types.String `tfsdk:"unit"`
	IsEnabled   types.Bool   `tfsdk:"is_enabled"`
}

func NewBillingCycleDataSource() datasource.DataSource {
	return &billingCycleDataSource{}
}

func (d *billingCycleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_billing_cycle"
}

func (d *billingCycleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a ZCP billing cycle by slug.",
		Attributes: map[string]schema.Attribute{
			"slug": schema.StringAttribute{
				MarkdownDescription: "Billing cycle slug (e.g. `hourly`, `monthly`).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Billing cycle ID.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Billing cycle display name.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Billing cycle description.",
				Computed:            true,
			},
			"duration": schema.Int64Attribute{
				MarkdownDescription: "Cycle duration in `unit`s.",
				Computed:            true,
			},
			"unit": schema.StringAttribute{
				MarkdownDescription: "Duration unit (e.g. `hour`, `month`).",
				Computed:            true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the billing cycle is enabled.",
				Computed:            true,
			},
		},
	}
}

func (d *billingCycleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	d.svc = billingcycle.NewService(pd.Client)
}

func (d *billingCycleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state billingCycleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_billing_cycle cannot be read: the provider was not configured successfully. Ensure bearer_token is set.")
		return
	}

	cycles, err := d.svc.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list billing cycles", err.Error())
		return
	}

	slug := state.Slug.ValueString()
	for _, c := range cycles {
		if c.Slug == slug {
			state.ID = types.StringValue(c.ID)
			state.Name = types.StringValue(c.Name)
			state.Description = types.StringValue(c.Description)
			state.Duration = types.Int64Value(int64(c.Duration))
			state.Unit = types.StringValue(c.Unit)
			state.IsEnabled = types.BoolValue(c.IsEnabled)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.Diagnostics.AddError(
		"Billing cycle not found",
		fmt.Sprintf("No billing cycle with slug %q exists.", slug),
	)
}
