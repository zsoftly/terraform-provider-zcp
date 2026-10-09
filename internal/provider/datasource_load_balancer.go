package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/loadbalancer"
)

var _ datasource.DataSource = &loadBalancerDataSource{}

type loadBalancerGetter interface {
	Get(ctx context.Context, slug string) (*loadbalancer.LoadBalancer, error)
}

type loadBalancerDataSource struct {
	svc loadBalancerGetter
}

type loadBalancerDataSourceModel struct {
	Slug          types.String `tfsdk:"slug"`
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	State         types.String `tfsdk:"state"`
	PublicIP      types.String `tfsdk:"public_ip"`
	Region        types.String `tfsdk:"region"`
	Project       types.String `tfsdk:"project"`
	CloudProvider types.String `tfsdk:"cloud_provider"`
	Rules         types.List   `tfsdk:"rules"`
}

func NewLoadBalancerDataSource() datasource.DataSource {
	return &loadBalancerDataSource{}
}

func (d *loadBalancerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_load_balancer"
}

func (d *loadBalancerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing ZCP load balancer by slug.",
		Attributes: map[string]schema.Attribute{
			"slug":           schema.StringAttribute{MarkdownDescription: "Load balancer slug.", Required: true},
			"id":             schema.StringAttribute{MarkdownDescription: "Load balancer slug (same as `slug`).", Computed: true},
			"name":           schema.StringAttribute{MarkdownDescription: "Load balancer display name.", Computed: true},
			"state":          schema.StringAttribute{MarkdownDescription: "Current load balancer state.", Computed: true},
			"public_ip":      schema.StringAttribute{MarkdownDescription: "Public IP address bound to the load balancer, when present.", Computed: true},
			"region":         schema.StringAttribute{MarkdownDescription: "Region slug, when returned by the API.", Computed: true},
			"project":        schema.StringAttribute{MarkdownDescription: "Project slug, when returned by the API.", Computed: true},
			"cloud_provider": schema.StringAttribute{MarkdownDescription: "Cloud provider slug, when returned by the API.", Computed: true},
			"rules": schema.ListNestedAttribute{
				MarkdownDescription: "Load balancing rules returned with the load balancer.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":            schema.StringAttribute{MarkdownDescription: "Rule ID, usable in `zcp_load_balancer_attachment`.", Computed: true},
					"name":          schema.StringAttribute{MarkdownDescription: "Rule name.", Computed: true},
					"public_port":   schema.StringAttribute{MarkdownDescription: "Rule public port.", Computed: true},
					"private_port":  schema.StringAttribute{MarkdownDescription: "Rule private port.", Computed: true},
					"protocol":      schema.StringAttribute{MarkdownDescription: "Rule protocol.", Computed: true},
					"algorithm":     schema.StringAttribute{MarkdownDescription: "Rule balancing algorithm.", Computed: true},
					"sticky_method": schema.StringAttribute{MarkdownDescription: "Rule session stickiness method.", Computed: true},
					"enable_tls":    schema.BoolAttribute{MarkdownDescription: "Whether TLS is enabled for the rule.", Computed: true},
					"enable_proxy":  schema.BoolAttribute{MarkdownDescription: "Whether the PROXY protocol is enabled for the rule.", Computed: true},
				}},
			},
		},
	}
}

func (d *loadBalancerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	d.svc = loadbalancer.NewService(pd.Client)
}

func (d *loadBalancerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state loadBalancerDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_load_balancer cannot be read: the provider was not configured successfully. Ensure bearer_token is set.")
		return
	}

	lb, err := d.svc.Get(ctx, state.Slug.ValueString())
	if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
		resp.Diagnostics.AddError("Load balancer not found", fmt.Sprintf("No load balancer with slug %q exists.", state.Slug.ValueString()))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read load balancer", err.Error())
		return
	}

	state.ID = types.StringValue(lb.Slug)
	state.Name = types.StringValue(lb.Name)
	state.State = types.StringValue(lb.State)
	state.PublicIP = nestedIPAddress(lb.IPAddress)
	state.Region = nestedRegion(lb.Region)
	state.Project = nestedProject(lb.Project)
	state.CloudProvider = nestedCloudProvider(lb.CloudProvider)
	state.Rules = loadBalancerRulesValue(lb.Rules)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func nestedIPAddress(ip *loadbalancer.IPAddress) types.String {
	if ip == nil || ip.IPAddress == "" {
		return types.StringNull()
	}
	return types.StringValue(ip.IPAddress)
}

func nestedRegion(region *loadbalancer.Region) types.String {
	if region == nil || region.Slug == "" {
		return types.StringNull()
	}
	return types.StringValue(region.Slug)
}

func nestedProject(project *loadbalancer.Project) types.String {
	if project == nil || project.Slug == "" {
		return types.StringNull()
	}
	return types.StringValue(project.Slug)
}

func nestedCloudProvider(provider *loadbalancer.CloudProvider) types.String {
	if provider == nil || provider.Slug == "" {
		return types.StringNull()
	}
	return types.StringValue(provider.Slug)
}

func loadBalancerRulesValue(rules []loadbalancer.Rule) types.List {
	ruleType := types.ObjectType{AttrTypes: loadBalancerRuleAttrTypes}
	values := make([]attr.Value, 0, len(rules))
	for _, rule := range rules {
		values = append(values, types.ObjectValueMust(loadBalancerRuleAttrTypes, map[string]attr.Value{
			"id":            types.StringValue(rule.ID),
			"name":          types.StringValue(rule.Name),
			"public_port":   types.StringValue(rule.PublicPort),
			"private_port":  types.StringValue(rule.PrivatePort),
			"protocol":      types.StringValue(rule.Protocol),
			"algorithm":     types.StringValue(rule.Algorithm),
			"sticky_method": types.StringValue(rule.StickyMethod),
			"enable_tls":    types.BoolValue(rule.EnableTLSProtocol),
			"enable_proxy":  types.BoolValue(rule.EnableProxyProtocol),
		}))
	}
	return types.ListValueMust(ruleType, values)
}

var loadBalancerRuleAttrTypes = map[string]attr.Type{
	"id":            types.StringType,
	"name":          types.StringType,
	"public_port":   types.StringType,
	"private_port":  types.StringType,
	"protocol":      types.StringType,
	"algorithm":     types.StringType,
	"sticky_method": types.StringType,
	"enable_tls":    types.BoolType,
	"enable_proxy":  types.BoolType,
}
