package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/iso"
)

var _ resource.Resource = &isoResource{}
var _ resource.ResourceWithConfigure = &isoResource{}
var _ resource.ResourceWithImportState = &isoResource{}

type isoServiceIface interface {
	List(ctx context.Context, regionSlug string) ([]iso.ISO, error)
	Create(ctx context.Context, req iso.CreateRequest) (*iso.ISO, error)
	Update(ctx context.Context, slug string, req iso.UpdateRequest) error
	Delete(ctx context.Context, slug string) error
}

type isoResource struct {
	svc            isoServiceIface
	defaultProject string
}

type isoResourceModel struct {
	ID              types.String   `tfsdk:"id"`
	Name            types.String   `tfsdk:"name"`
	Description     types.String   `tfsdk:"description"`
	URL             types.String   `tfsdk:"url"`
	CloudProvider   types.String   `tfsdk:"cloud_provider"`
	Region          types.String   `tfsdk:"region"`
	Project         types.String   `tfsdk:"project"`
	OSTypeID        types.String   `tfsdk:"os_type_id"`
	OperatingSystem types.String   `tfsdk:"operating_system"`
	OSVersion       types.String   `tfsdk:"operating_system_version"`
	BillingCycle    types.String   `tfsdk:"billing_cycle"`
	PasswordEnabled types.Bool     `tfsdk:"password_enabled"`
	IsExtractable   types.Bool     `tfsdk:"is_extractable"`
	IsBootable      types.Bool     `tfsdk:"is_bootable"`
	State           types.String   `tfsdk:"state"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
}

func NewISOResource() resource.Resource {
	return &isoResource{}
}

func (r *isoResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iso"
}

func (r *isoResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Registers an ISO image from a URL. The permission flags (`password_enabled`, `is_extractable`, `is_bootable`) update in place; every other change forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ISO slug.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ISO display name. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Human-readable description. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "HTTP(S) URL the platform downloads the ISO from. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"cloud_provider": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cloud provider slug (e.g. `zsoftly`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Region slug (e.g. `yow-1`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project slug. Inherits from the provider `default_project` if omitted. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"os_type_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Operating system type ID. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{requiresReplaceUnlessAdopting()},
			},
			"operating_system": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Operating system name (e.g. `Ubuntu`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{requiresReplaceUnlessAdopting()},
			},
			"operating_system_version": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Operating system version (e.g. `24.04`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{requiresReplaceUnlessAdopting()},
			},
			"billing_cycle": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Billing cycle (e.g. `hourly`, `monthly`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"password_enabled": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether password reset is supported. Updated in place.",
			},
			"is_extractable": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether the ISO is downloadable by other users. Updated in place.",
			},
			"is_bootable": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether the ISO is bootable. Updated in place.",
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current state of the ISO.",
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

func (r *isoResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = iso.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

func (r *isoResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model isoResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_iso cannot be created: bearer_token is missing.")
		return
	}

	createTimeout, diags := model.Timeouts.Create(ctx, 30*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	project := r.defaultProject
	if !model.Project.IsNull() && !model.Project.IsUnknown() {
		project = model.Project.ValueString()
	}

	createReq := iso.CreateRequest{
		Name:                   model.Name.ValueString(),
		URL:                    model.URL.ValueString(),
		CloudProvider:          model.CloudProvider.ValueString(),
		Project:                project,
		Region:                 model.Region.ValueString(),
		OSTypeID:               model.OSTypeID.ValueString(),
		ImageType:              "ISO",
		OperatingSystem:        model.OperatingSystem.ValueString(),
		OperatingSystemVersion: model.OSVersion.ValueString(),
		BillingCycle:           model.BillingCycle.ValueString(),
		Service:                "iso",
	}
	if !model.Description.IsNull() && !model.Description.IsUnknown() {
		createReq.Description = model.Description.ValueString()
	}
	if !model.PasswordEnabled.IsNull() && !model.PasswordEnabled.IsUnknown() {
		createReq.PasswordEnabled = model.PasswordEnabled.ValueBool()
	}
	if !model.IsExtractable.IsNull() && !model.IsExtractable.IsUnknown() {
		createReq.IsExtractable = model.IsExtractable.ValueBool()
	}
	// is_bootable defaults to true: a non-bootable ISO is the rare case.
	createReq.IsBootable = model.IsBootable.IsNull() || model.IsBootable.IsUnknown() || model.IsBootable.ValueBool()

	created, err := r.svc.Create(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create ISO", err.Error())
		return
	}

	model.ID = types.StringValue(created.Slug)
	if created.State != "" {
		model.State = types.StringValue(created.State)
	} else {
		model.State = types.StringNull()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *isoResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model isoResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_iso cannot be read: bearer_token is missing.")
		return
	}

	isos, err := r.svc.List(ctx, model.Region.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read ISO", err.Error())
		return
	}

	slug := model.ID.ValueString()
	for _, img := range isos {
		if img.Slug == slug {
			model.Name = types.StringValue(img.Name)
			if img.Description != "" {
				model.Description = types.StringValue(img.Description)
			}
			if img.ISOURL != "" {
				model.URL = types.StringValue(img.ISOURL)
			}
			if img.State != "" {
				model.State = types.StringValue(img.State)
			}
			if !model.PasswordEnabled.IsNull() {
				model.PasswordEnabled = types.BoolValue(img.PasswordEnabled)
			}
			if !model.IsExtractable.IsNull() {
				model.IsExtractable = types.BoolValue(img.IsExtractable)
			}
			if !model.IsBootable.IsNull() {
				model.IsBootable = types.BoolValue(img.IsBootable)
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *isoResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state isoResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_iso cannot be updated: bearer_token is missing.")
		return
	}

	updateTimeout, diags := plan.Timeouts.Update(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	updateReq := iso.UpdateRequest{
		PasswordEnabled: !plan.PasswordEnabled.IsNull() && plan.PasswordEnabled.ValueBool(),
		IsExtractable:   !plan.IsExtractable.IsNull() && plan.IsExtractable.ValueBool(),
		IsBootable:      plan.IsBootable.IsNull() || plan.IsBootable.ValueBool(),
	}
	if err := r.svc.Update(ctx, state.ID.ValueString(), updateReq); err != nil {
		resp.Diagnostics.AddError("Failed to update ISO permissions", err.Error())
		return
	}

	model := plan
	model.ID = state.ID
	model.State = state.State
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *isoResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model isoResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_iso cannot be deleted: bearer_token is missing.")
		return
	}

	deleteTimeout, diags := model.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	slug := model.ID.ValueString()
	err := r.svc.Delete(deleteCtx, slug)
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete ISO", err.Error())
		return
	}

	region := model.Region.ValueString()
	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		isos, err := r.svc.List(ctx, region)
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for _, img := range isos {
			if img.Slug == slug {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError("ISO deletion did not complete", err.Error())
	}
}

// ImportState accepts a composite ID so the write-only scope fields are seeded
// for a zero-diff plan after import. Format:
//
//	<slug>/<region>/<cloud_provider>/<billing_cycle>[/<project>]
func (r *isoResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"id", "region", "cloud_provider", "billing_cycle", "project"}
	importPositional(ctx, req, resp, fields, 4, "<slug>/<region>/<cloud_provider>/<billing_cycle>[/<project>]")
}
