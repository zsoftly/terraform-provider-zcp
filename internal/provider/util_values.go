package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/loadbalancer"
)

// stateOrNull maps an API state string to Terraform state: the value when the
// API reported one, null when it did not.
func stateOrNull(s string) types.String {
	if s != "" {
		return types.StringValue(s)
	}
	return types.StringNull()
}

// buildCreateRuleSpec assembles the rule spec shared by zcp_load_balancer
// (initial rule) and zcp_load_balancer_rule, so the two cannot drift.
func buildCreateRuleSpec(name, publicPort, privatePort, protocol, algorithm, stickyMethod string, enableTLS, enableProxy types.Bool) loadbalancer.CreateRuleSpec {
	spec := loadbalancer.CreateRuleSpec{
		Name:            name,
		PublicPort:      publicPort,
		PrivatePort:     privatePort,
		Protocol:        protocol,
		Algorithm:       algorithm,
		StickyMethod:    stickyMethod,
		VirtualMachines: []loadbalancer.VMAttachment{},
	}
	if !enableTLS.IsNull() && !enableTLS.IsUnknown() {
		spec.EnableTLSProtocol = enableTLS.ValueBool()
	}
	if !enableProxy.IsNull() && !enableProxy.IsUnknown() {
		spec.EnableProxyProtocol = enableProxy.ValueBool()
	}
	return spec
}
