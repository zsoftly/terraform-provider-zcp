package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/plan"
)

var _ datasource.DataSource = &planDataSource{}

type planLister interface {
	List(ctx context.Context, svc plan.ServiceType) ([]plan.Plan, error)
}

type planDataSource struct {
	svc planLister
}

type planDataSourceModel struct {
	Slug         types.String  `tfsdk:"slug"`
	Service      types.String  `tfsdk:"service"`
	ID           types.String  `tfsdk:"id"`
	Name         types.String  `tfsdk:"name"`
	CPU          types.Int64   `tfsdk:"cpu"`
	MemoryMB     types.Int64   `tfsdk:"memory_mb"`
	StorageGB    types.Int64   `tfsdk:"storage_gb"`
	HourlyPrice  types.Float64 `tfsdk:"hourly_price"`
	MonthlyPrice types.Float64 `tfsdk:"monthly_price"`
}

func NewPlanDataSource() datasource.DataSource {
	return &planDataSource{}
}

func (d *planDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plan"
}

func (d *planDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a ZCP service plan by slug.",
		Attributes: map[string]schema.Attribute{
			"slug": schema.StringAttribute{
				MarkdownDescription: "Unique plan slug (e.g. `ci1xs`).",
				Required:            true,
			},
			"service": schema.StringAttribute{
				MarkdownDescription: "Service type to search (e.g. `Virtual Machine`, `Kubernetes`). Defaults to `Virtual Machine`.",
				Optional:            true,
				Validators:          []validator.String{planServiceTypeValidator{}},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Plan ID.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Plan display name.",
				Computed:            true,
			},
			"cpu": schema.Int64Attribute{
				MarkdownDescription: "Number of vCPUs.",
				Computed:            true,
			},
			"memory_mb": schema.Int64Attribute{
				MarkdownDescription: "RAM in megabytes.",
				Computed:            true,
			},
			"storage_gb": schema.Int64Attribute{
				MarkdownDescription: "Root disk size in gigabytes.",
				Computed:            true,
			},
			"hourly_price": schema.Float64Attribute{
				MarkdownDescription: "Hourly price in account currency.",
				Computed:            true,
			},
			"monthly_price": schema.Float64Attribute{
				MarkdownDescription: "Monthly price in account currency.",
				Computed:            true,
			},
		},
	}
}

func (d *planDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
	d.svc = plan.NewService(pd.Client)
}

func (d *planDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state planDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.svc == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"zcp_plan cannot be read: the provider was not configured successfully. Ensure bearer_token is set.",
		)
		return
	}

	svcType := plan.ServiceVM
	if !state.Service.IsNull() && !state.Service.IsUnknown() {
		svcType = plan.ServiceType(state.Service.ValueString())
	}

	plans, err := d.svc.List(ctx, svcType)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list plans", err.Error())
		return
	}

	slug := state.Slug.ValueString()
	for _, p := range plans {
		if p.Slug == slug {
			cpuF, cpuErr := p.Attribute.CPU.Float64()
			memF, memErr := p.Attribute.Memory.Float64()
			storF, storErr := p.Attribute.Storage.Float64()
			if cpuErr != nil || memErr != nil || storErr != nil {
				resp.Diagnostics.AddError(
					"Invalid plan attribute",
					fmt.Sprintf("Cannot convert plan %q attributes to numbers (cpu=%s, memory=%s, storage=%s).",
						slug, p.Attribute.CPU, p.Attribute.Memory, p.Attribute.Storage),
				)
				return
			}

			state.ID = types.StringValue(p.ID)
			state.Name = types.StringValue(p.Name)
			state.CPU = types.Int64Value(int64(cpuF))
			state.MemoryMB = types.Int64Value(int64(memF))
			state.StorageGB = types.Int64Value(int64(storF))
			state.HourlyPrice = types.Float64Value(p.HourlyPrice)
			state.MonthlyPrice = types.Float64Value(p.MonthlyPrice)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Plan not found",
		fmt.Sprintf("No %s plan with slug %q exists.", svcType, slug),
	)
}
