package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/vpc"
)

var _ resource.Resource = &vpcResource{}
var _ resource.ResourceWithConfigure = &vpcResource{}
var _ resource.ResourceWithImportState = &vpcResource{}

type vpcServiceIface interface {
	List(ctx context.Context, zoneSlug, region, project string) ([]vpc.VPC, error)
	Get(ctx context.Context, slug string) (*vpc.VPC, error)
	Create(ctx context.Context, req vpc.CreateRequest) (*vpc.VPC, error)
	Update(ctx context.Context, slug string, req vpc.UpdateRequest) (*vpc.VPC, error)
	Delete(ctx context.Context, slug string) error
}

type vpcResource struct {
	svc            vpcServiceIface
	defaultProject string
}

type vpcResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	CloudProvider   types.String `tfsdk:"cloud_provider"`
	Region          types.String `tfsdk:"region"`
	CIDR            types.String `tfsdk:"cidr"`
	Size            types.String `tfsdk:"size"`
	Project         types.String `tfsdk:"project"`
	Description     types.String `tfsdk:"description"`
	VPCType         types.String `tfsdk:"type"`
	BillingCycle    types.String `tfsdk:"billing_cycle"`
	Plan            types.String `tfsdk:"plan"`
	StorageCategory types.String `tfsdk:"storage_category"`
	// Computed
	Status   types.String   `tfsdk:"status"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func NewVPCResource() resource.Resource {
	return &vpcResource{}
}

func (r *vpcResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vpc"
}

func (r *vpcResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP Virtual Private Cloud (VPC).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "VPC slug (unique identifier).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name for the VPC.",
			},
			"cloud_provider": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cloud provider slug (e.g. `nimbo`). Use `data.zcp_region.<name>.cloud_provider` instead of hardcoding.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Region slug where the VPC is created (e.g. `yow-1`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"cidr": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Network address for the VPC (e.g. `10.1.0.1`). This is the base IP address, not CIDR notation — do not include the prefix length.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"size": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Subnet mask prefix length as a string (e.g. `\"24\"` for /24, `\"16\"` for /16).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project slug. Inherits from the provider `default_project` if omitted.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Human-readable description.",
			},
			"type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "VPC type (e.g. `Isolated`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"billing_cycle": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Billing cycle (e.g. `monthly`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"plan": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Plan slug for VPC compute resources (e.g. `virtual-private-cloud-vpc-1`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"storage_category": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Storage category slug (e.g. `nvme`, `pro-nvme`). Required by the API when creating a VPC.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current VPC status.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
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

func (r *vpcResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = vpc.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

func (r *vpcResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model vpcResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vpc cannot be created: bearer_token is missing.")
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

	v, err := r.svc.Create(ctx, vpc.CreateRequest{
		Name:            model.Name.ValueString(),
		CloudProvider:   model.CloudProvider.ValueString(),
		Region:          model.Region.ValueString(),
		Project:         project,
		CIDR:            model.CIDR.ValueString(),
		Size:            model.Size.ValueString(),
		Description:     model.Description.ValueString(),
		Type:            model.VPCType.ValueString(),
		BillingCycle:    model.BillingCycle.ValueString(),
		Plan:            model.Plan.ValueString(),
		StorageCategory: model.StorageCategory.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create VPC", err.Error())
		return
	}

	model.ID = types.StringValue(v.Slug)
	// Always set a concrete status value to satisfy the framework requirement that
	// no unknown values remain after apply. Status may be empty initially (VPC still
	// provisioning) and populated after the next refresh.
	model.Status = types.StringValue(v.Status)
	// description is Optional+Computed: resolve it to a known value (the API echo,
	// or "" when omitted) so no unknown remains after apply.
	model.Description = apiOrKeep(model.Description, v.Description)
	// Do NOT overwrite cidr from the API response: the API echoes it back in
	// prefixed form (e.g. "10.7.0.1/24") while this attribute is the base address
	// the user supplied ("10.7.0.1"). Overwriting produces an inconsistent-result
	// error and a perpetual diff. The plan value is authoritative.
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *vpcResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model vpcResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vpc cannot be read: bearer_token is missing.")
		return
	}

	project := r.defaultProject
	if !model.Project.IsNull() && !model.Project.IsUnknown() {
		project = model.Project.ValueString()
	}
	vpcs, err := r.svc.List(ctx, "", model.Region.ValueString(), project)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read VPC", err.Error())
		return
	}

	slug := model.ID.ValueString()
	for _, v := range vpcs {
		if v.Slug == slug {
			model.Name = types.StringValue(v.Name)
			// description and status are not included in the list response;
			// only update from API when the server returns a non-empty value.
			if v.Description != "" {
				model.Description = types.StringValue(v.Description)
			}
			if v.Status != "" {
				model.Status = types.StringValue(v.Status)
			}
			// cidr is intentionally NOT refreshed from the API: it returns the
			// prefixed form ("10.7.0.1/24") whereas the attribute holds the base
			// address the user set ("10.7.0.1"). Preserving state avoids a perpetual diff.
			// cloud_provider, region, project, type, billing_cycle, plan, size are
			// write-only (not in API response); preserved from state.
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *vpcResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model vpcResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state vpcResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vpc cannot be updated: bearer_token is missing.")
		return
	}

	desc := model.Description.ValueString()
	_, err := r.svc.Update(ctx, state.ID.ValueString(), vpc.UpdateRequest{
		Name:        model.Name.ValueString(),
		Description: &desc,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to update VPC", err.Error())
		return
	}

	// Merge: plan has the new name/description; preserve computed and write-only from state.
	model.ID = state.ID
	model.Status = state.Status
	model.CIDR = state.CIDR
	model.CloudProvider = state.CloudProvider
	model.Region = state.Region
	model.Size = state.Size
	model.VPCType = state.VPCType
	model.BillingCycle = state.BillingCycle
	model.Plan = state.Plan
	model.StorageCategory = state.StorageCategory
	model.Project = state.Project
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *vpcResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model vpcResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vpc cannot be deleted: bearer_token is missing.")
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
	if err != nil && !apierrors.IsNotFound(err) && !apierrors.IsResourceNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete VPC", err.Error())
		return
	}

	if err := pollUntilGone(deleteCtx, 5*time.Second, func(ctx context.Context) (bool, error) {
		_, err := r.svc.Get(ctx, slug)
		// vpc.Get returns the package sentinel vpc.ErrNotFound (not an *APIError)
		// when the VPC is gone, so check it alongside the HTTP not-found helpers —
		// otherwise a successful delete is reported as "deletion did not complete".
		if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) || errors.Is(err, vpc.ErrNotFound) {
			return false, nil
		}
		return err == nil, err
	}); err != nil {
		resp.Diagnostics.AddError("VPC deletion did not complete", err.Error())
	}
}

// ImportState accepts a composite ID so the write-only / create-only attributes
// the API never returns (and cidr, which it returns in a different prefixed form)
// can be seeded for a zero-diff plan after import. Format (slash-separated,
// trailing/empty segments allowed):
//
//	<slug>/<cloud_provider>/<region>/<cidr>/<size>/<project>/<storage_category>/<type>/<billing_cycle>/<plan>/<description>
//
// <slug>/<cloud_provider>/<region>/<cidr>/<size> are required (all RequiresReplace
// and not refreshed from the API). <description> is last so a value containing "/"
// survives. name and status come from the subsequent Read.
func (r *vpcResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"id", "cloud_provider", "region", "cidr", "size", "project", "storage_category", "type", "billing_cycle", "plan", "description"}
	importPositional(ctx, req, resp, fields, 5,
		"<slug>/<cloud_provider>/<region>/<cidr>/<size>[/<project>/<storage_category>/<type>/<billing_cycle>/<plan>/<description>]")
}
