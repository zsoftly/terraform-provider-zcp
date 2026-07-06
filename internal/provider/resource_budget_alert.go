package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/billing"
)

// budgetAlertID is the fixed ID of the account's single budget alert setting.
const budgetAlertID = "budget-alert"

var _ resource.Resource = &budgetAlertResource{}
var _ resource.ResourceWithConfigure = &budgetAlertResource{}
var _ resource.ResourceWithImportState = &budgetAlertResource{}

type budgetAlertServiceIface interface {
	GetBudgetAlert(ctx context.Context) (json.RawMessage, error)
	SetBudgetAlert(ctx context.Context, req billing.SetBudgetAlertRequest) (json.RawMessage, error)
}

type budgetAlertResource struct {
	svc budgetAlertServiceIface
}

type budgetAlertResourceModel struct {
	ID        types.String  `tfsdk:"id"`
	Amount    types.Float64 `tfsdk:"amount"`
	Threshold types.Float64 `tfsdk:"threshold"`
	Enabled   types.Bool    `tfsdk:"enabled"`
}

func NewBudgetAlertResource() resource.Resource {
	return &budgetAlertResource{}
}

func (r *budgetAlertResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_budget_alert"
}

func (r *budgetAlertResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the account's budget alert settings. The account has a single budget " +
			"alert, so declare at most one of this resource. Destroy disables the alert without clearing " +
			"the configured amounts.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Fixed identifier (`" + budgetAlertID + "`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"amount": schema.Float64Attribute{
				Required:            true,
				MarkdownDescription: "Monthly budget amount in account currency. Updated in place.",
			},
			"threshold": schema.Float64Attribute{
				Required:            true,
				MarkdownDescription: "Alert threshold as a percentage of the budget (e.g. `80`). Updated in place.",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether the alert is active. Defaults to `true`. Updated in place.",
			},
		},
	}
}

func (r *budgetAlertResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = billing.NewService(pd.Client)
}

func (r *budgetAlertResource) enabledOrDefault(enabled types.Bool) bool {
	return enabled.IsNull() || enabled.IsUnknown() || enabled.ValueBool()
}

func (r *budgetAlertResource) apply(ctx context.Context, model *budgetAlertResourceModel) error {
	_, err := r.svc.SetBudgetAlert(ctx, billing.SetBudgetAlertRequest{
		Amount:    model.Amount.ValueFloat64(),
		Threshold: model.Threshold.ValueFloat64(),
		IsEnabled: r.enabledOrDefault(model.Enabled),
	})
	return err
}

func (r *budgetAlertResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model budgetAlertResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_budget_alert cannot be created: bearer_token is missing.")
		return
	}

	if err := r.apply(ctx, &model); err != nil {
		resp.Diagnostics.AddError("Failed to set budget alert", err.Error())
		return
	}

	model.ID = types.StringValue(budgetAlertID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *budgetAlertResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model budgetAlertResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_budget_alert cannot be read: bearer_token is missing.")
		return
	}

	raw, err := r.svc.GetBudgetAlert(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read budget alert", err.Error())
		return
	}

	var settings billing.BudgetAlert
	if len(raw) > 0 && string(raw) != "null" {
		if uerr := json.Unmarshal(raw, &settings); uerr != nil {
			resp.Diagnostics.AddError("Failed to decode budget alert settings", uerr.Error())
			return
		}
		model.Amount = types.Float64Value(settings.Amount)
		model.Threshold = types.Float64Value(settings.Threshold)
		if !model.Enabled.IsNull() || !settings.IsEnabled {
			model.Enabled = types.BoolValue(settings.IsEnabled)
		}
	}

	model.ID = types.StringValue(budgetAlertID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *budgetAlertResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model budgetAlertResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_budget_alert cannot be updated: bearer_token is missing.")
		return
	}

	if err := r.apply(ctx, &model); err != nil {
		resp.Diagnostics.AddError("Failed to update budget alert", err.Error())
		return
	}

	model.ID = types.StringValue(budgetAlertID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// Delete disables the alert; the API has no way to remove the setting entirely.
func (r *budgetAlertResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model budgetAlertResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_budget_alert cannot be deleted: bearer_token is missing.")
		return
	}

	if _, err := r.svc.SetBudgetAlert(ctx, billing.SetBudgetAlertRequest{
		Amount:    model.Amount.ValueFloat64(),
		Threshold: model.Threshold.ValueFloat64(),
		IsEnabled: false,
	}); err != nil {
		resp.Diagnostics.AddError("Failed to disable budget alert", err.Error())
	}
}

func (r *budgetAlertResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The account has one budget alert; any import ID maps to the fixed ID.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), budgetAlertID)...)
}
