package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/acl"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
)

var _ resource.Resource = &networkACLResource{}
var _ resource.ResourceWithConfigure = &networkACLResource{}
var _ resource.ResourceWithImportState = &networkACLResource{}

// aclServiceIface is the subset of the ACL API used by the ACL, ACL-rule, and
// network resources.
type aclServiceIface interface {
	List(ctx context.Context, vpcSlug string) ([]acl.NetworkACL, error)
	Create(ctx context.Context, vpcSlug string, req acl.ACLCreateRequest) error
	Delete(ctx context.Context, vpcSlug, aclID string) error
	ListRules(ctx context.Context, vpcSlug, aclID string) ([]acl.Rule, error)
	CreateRule(ctx context.Context, vpcSlug, aclID string, req acl.RuleCreateRequest) error
	UpdateRule(ctx context.Context, vpcSlug, aclID, ruleID string, req acl.RuleCreateRequest) error
	DeleteRule(ctx context.Context, vpcSlug, aclID, ruleID string) error
	ReplaceNetworkACL(ctx context.Context, networkSlug, aclID string) error
}

type networkACLResource struct {
	svc aclServiceIface
}

type networkACLResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	VPC         types.String `tfsdk:"vpc"`
	Description types.String `tfsdk:"description"`
}

func NewNetworkACLResource() resource.Resource {
	return &networkACLResource{}
}

func (r *networkACLResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_acl"
}

func (r *networkACLResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZCP Network ACL, a stateful allow/deny rule set inside a VPC. The ACL tracks connections and accepts replies to allowed traffic automatically. Ingress and egress rules do not correlate, so an inbound listener still needs an explicit ingress rule. Attach the ACL to a subnet with the `acl` argument on `zcp_network`, and add rules with `zcp_network_acl_rule`. The ACL has no update endpoint, so changing any attribute forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Network ACL ID (UUID).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name for the ACL. Must be unique within the VPC. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"vpc": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Slug of the `zcp_vpc` this ACL belongs to. Changing this forces replacement.",
				PlanModifiers:       requiresReplace,
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Human-readable description. The API requires one; defaults to `name` when omitted. Changing this forces replacement (the API has no ACL update endpoint).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *networkACLResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = acl.NewService(pd.Client)
}

func (r *networkACLResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model networkACLResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_network_acl cannot be created: bearer_token is missing.")
		return
	}

	vpcSlug := model.VPC.ValueString()
	name := model.Name.ValueString()
	// The API requires a non-empty description; default it to the name.
	desc := model.Description.ValueString()
	if desc == "" {
		desc = name
	}
	if err := r.svc.Create(ctx, vpcSlug, acl.ACLCreateRequest{
		Name:        name,
		Description: desc,
		VPC:         vpcSlug,
	}); err != nil {
		resp.Diagnostics.AddError("Failed to create network ACL", err.Error())
		return
	}
	model.Description = types.StringValue(desc)

	// The create endpoint does not return the new ACL, so resolve its ID by name
	// (names are unique within a VPC).
	id, err := r.resolveACLID(ctx, vpcSlug, name)
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve created ACL", err.Error())
		return
	}
	model.ID = types.StringValue(id)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *networkACLResource) resolveACLID(ctx context.Context, vpcSlug, name string) (string, error) {
	acls, err := r.svc.List(ctx, vpcSlug)
	if err != nil {
		return "", err
	}
	for _, a := range acls {
		if a.Name == name && a.ID != "" {
			return a.ID, nil
		}
	}
	return "", fmt.Errorf("ACL %q not found in VPC %q after creation", name, vpcSlug)
}

func (r *networkACLResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model networkACLResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_network_acl cannot be read: bearer_token is missing.")
		return
	}

	acls, err := r.svc.List(ctx, model.VPC.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read network ACL", err.Error())
		return
	}
	id := model.ID.ValueString()
	for _, a := range acls {
		if a.ID == id {
			model.Name = types.StringValue(a.Name)
			if a.Description != "" {
				model.Description = types.StringValue(a.Description)
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

// Update is a no-op: every attribute is RequiresReplace (the API has no ACL
// update endpoint), so the framework never calls this with a real diff.
func (r *networkACLResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model networkACLResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *networkACLResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model networkACLResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_network_acl cannot be deleted: bearer_token is missing.")
		return
	}
	if err := r.svc.Delete(ctx, model.VPC.ValueString(), model.ID.ValueString()); err != nil &&
		!apierrors.IsNotFound(err) && !apierrors.IsResourceNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete network ACL", err.Error())
	}
}

// ImportState accepts "<vpc-slug>/<acl-id>" so the VPC (needed for every ACL API
// call) can be seeded alongside the ACL ID.
func (r *networkACLResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fields := []string{"vpc", "id"}
	importPositional(ctx, req, resp, fields, 2, "<vpc-slug>/<acl-id>")
}
