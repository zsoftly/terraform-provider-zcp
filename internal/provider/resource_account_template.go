package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/template"
)

var _ resource.Resource = &accountTemplateResource{}
var _ resource.ResourceWithConfigure = &accountTemplateResource{}
var _ resource.ResourceWithImportState = &accountTemplateResource{}
var _ resource.ResourceWithValidateConfig = &accountTemplateResource{}

type accountTemplateServiceIface interface {
	ListAccount(ctx context.Context) ([]template.AccountTemplate, error)
	CreateAccount(ctx context.Context, req template.CreateAccountTemplateRequest) (*template.AccountTemplate, error)
	DeleteAccount(ctx context.Context, slug string) error
}

type accountTemplateResource struct {
	svc            accountTemplateServiceIface
	defaultProject string
}

type accountTemplateResourceModel struct {
	ID              types.String   `tfsdk:"id"`
	Name            types.String   `tfsdk:"name"`
	Description     types.String   `tfsdk:"description"`
	URL             types.String   `tfsdk:"url"`
	VirtualMachine  types.String   `tfsdk:"virtual_machine"`
	CloudProvider   types.String   `tfsdk:"cloud_provider"`
	Region          types.String   `tfsdk:"region"`
	Project         types.String   `tfsdk:"project"`
	OSTypeID        types.String   `tfsdk:"os_type_id"`
	OperatingSystem types.String   `tfsdk:"operating_system"`
	OSVersion       types.String   `tfsdk:"operating_system_version"`
	BillingCycle    types.String   `tfsdk:"billing_cycle"`
	Format          types.String   `tfsdk:"format"`
	PasswordEnabled types.Bool     `tfsdk:"password_enabled"`
	State           types.String   `tfsdk:"state"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
}

func NewAccountTemplateResource() resource.Resource {
	return &accountTemplateResource{}
}

func (r *accountTemplateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_template"
}

func (r *accountTemplateResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Registers a custom template in the account catalogue, either from an image URL or " +
			"captured from an existing instance. Templates are immutable, so every change forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Account template slug.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Template display name. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Human-readable description. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "HTTP(S) URL of the source image. Exactly one of `url` or `virtual_machine` must be set. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"virtual_machine": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Slug of an instance to capture the template from. Exactly one of `url` or `virtual_machine` must be set. Changing this forces replacement.",
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
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"operating_system": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Operating system name (e.g. `Ubuntu`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"operating_system_version": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Operating system version (e.g. `24.04`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"billing_cycle": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Billing cycle (e.g. `hourly`, `monthly`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"format": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Image format (e.g. `QCOW2`). Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"password_enabled": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether password reset is supported. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current state of the template.",
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Delete: true,
			}),
		},
	}
}

func (r *accountTemplateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = template.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

// ValidateConfig enforces exactly one template source: url or virtual_machine.
func (r *accountTemplateResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var model accountTemplateResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	urlSet := !model.URL.IsNull() && !model.URL.IsUnknown()
	vmSet := !model.VirtualMachine.IsNull() && !model.VirtualMachine.IsUnknown()
	if urlSet == vmSet {
		resp.Diagnostics.AddError(
			"Invalid template source",
			"Set exactly one of `url` (register from an image URL) or `virtual_machine` (capture from an instance).",
		)
	}
}

func (r *accountTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model accountTemplateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_account_template cannot be created: bearer_token is missing.")
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

	createReq := template.CreateAccountTemplateRequest{
		Name:                   model.Name.ValueString(),
		CloudProvider:          model.CloudProvider.ValueString(),
		Region:                 model.Region.ValueString(),
		Project:                project,
		OSTypeID:               model.OSTypeID.ValueString(),
		ImageType:              "Template",
		OperatingSystem:        model.OperatingSystem.ValueString(),
		OperatingSystemVersion: model.OSVersion.ValueString(),
		BillingCycle:           model.BillingCycle.ValueString(),
	}
	if !model.Description.IsNull() && !model.Description.IsUnknown() {
		createReq.Description = model.Description.ValueString()
	}
	if !model.URL.IsNull() && !model.URL.IsUnknown() {
		createReq.URL = model.URL.ValueString()
	}
	if !model.VirtualMachine.IsNull() && !model.VirtualMachine.IsUnknown() {
		createReq.VirtualMachine = model.VirtualMachine.ValueString()
	}
	if !model.Format.IsNull() && !model.Format.IsUnknown() {
		createReq.Format = model.Format.ValueString()
	}
	if !model.PasswordEnabled.IsNull() && !model.PasswordEnabled.IsUnknown() {
		createReq.PasswordEnabled = model.PasswordEnabled.ValueBool()
	}

	created, err := r.svc.CreateAccount(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create account template", err.Error())
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

func (r *accountTemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model accountTemplateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_account_template cannot be read: bearer_token is missing.")
		return
	}

	templates, err := r.svc.ListAccount(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read account template", err.Error())
		return
	}

	slug := model.ID.ValueString()
	for _, t := range templates {
		if t.Slug == slug {
			model.Name = types.StringValue(t.Name)
			if t.Description != "" {
				model.Description = types.StringValue(t.Description)
			}
			if t.State != "" {
				model.State = types.StringValue(t.State)
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *accountTemplateResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are ForceNew; Terraform never invokes this method.
}

func (r *accountTemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model accountTemplateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_account_template cannot be deleted: bearer_token is missing.")
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
	err := r.svc.DeleteAccount(deleteCtx, slug)
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete account template", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		templates, err := r.svc.ListAccount(ctx)
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for _, t := range templates {
			if t.Slug == slug {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		resp.Diagnostics.AddError("Account template deletion did not complete", err.Error())
	}
}

// ImportState accepts a composite ID so the write-only scope fields are seeded
// for a zero-diff plan after import. Format:
//
//	<slug>/<region>/<cloud_provider>/<billing_cycle>[/<project>]
func (r *accountTemplateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"id", "region", "cloud_provider", "billing_cycle", "project"}
	importPositional(ctx, req, resp, fields, 4, "<slug>/<region>/<cloud_provider>/<billing_cycle>[/<project>]")
}
