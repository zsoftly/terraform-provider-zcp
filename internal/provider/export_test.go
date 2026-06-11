// export_test.go exposes package-internal constructors for use in
// external test packages. This file is only compiled during test builds.
package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// NewProjectDataSourceWithLister creates a projectDataSource pre-wired with
// the given lister; available only in test binaries.
func NewProjectDataSourceWithLister(l projectLister) datasource.DataSource {
	return &projectDataSource{svc: l}
}

// NewRegionDataSourceWithLister creates a regionDataSource pre-wired with
// the given lister; available only in test binaries.
func NewRegionDataSourceWithLister(l regionLister) datasource.DataSource {
	return &regionDataSource{svc: l}
}

// NewTemplateDataSourceWithLister creates a templateDataSource pre-wired with
// the given lister; available only in test binaries.
func NewTemplateDataSourceWithLister(l templateLister) datasource.DataSource {
	return &templateDataSource{svc: l}
}

// NewPlanDataSourceWithLister creates a planDataSource pre-wired with
// the given lister; available only in test binaries.
func NewPlanDataSourceWithLister(l planLister) datasource.DataSource {
	return &planDataSource{svc: l}
}

// NewSSHKeyResourceWithService creates an sshKeyResource pre-wired with the
// given service; available only in test binaries.
func NewSSHKeyResourceWithService(svc sshKeyServiceIface) resource.Resource {
	return &sshKeyResource{svc: svc}
}

// NewNetworkResourceWithService creates a networkResource pre-wired with the
// given service; available only in test binaries.
func NewNetworkResourceWithService(svc networkServiceIface) resource.Resource {
	return &networkResource{svc: svc}
}

// NewVPCResourceWithService creates a vpcResource pre-wired with the
// given service; available only in test binaries.
func NewVPCResourceWithService(svc vpcServiceIface) resource.Resource {
	return &vpcResource{svc: svc}
}

// NewVPNUserResourceWithService creates a vpnUserResource pre-wired with the
// given service; available only in test binaries.
func NewVPNUserResourceWithService(svc vpnUserServiceIface) resource.Resource {
	return &vpnUserResource{svc: svc}
}

// NewIPAddressResourceWithService creates an ipAddressResource pre-wired with the
// given service; available only in test binaries.
func NewIPAddressResourceWithService(svc ipAddressServiceIface) resource.Resource {
	return &ipAddressResource{svc: svc}
}

// NewVPCVPNGatewayResourceWithService creates a vpcVPNGatewayResource pre-wired
// with the given service; available only in test binaries.
func NewVPCVPNGatewayResourceWithService(svc vpcVPNGatewayServiceIface) resource.Resource {
	return &vpcVPNGatewayResource{svc: svc}
}

// NewPortForwardResourceWithService creates a portForwardResource pre-wired with
// the given service; available only in test binaries.
func NewPortForwardResourceWithService(svc portForwardServiceIface) resource.Resource {
	return &portForwardResource{svc: svc}
}

// NewVPNCustomerGatewayResourceWithService creates a vpnCustomerGatewayResource
// pre-wired with the given service; available only in test binaries.
func NewVPNCustomerGatewayResourceWithService(svc vpnCustomerGatewayServiceIface) resource.Resource {
	return &vpnCustomerGatewayResource{svc: svc}
}

// NewFirewallRuleResourceWithService creates a firewallRuleResource pre-wired with the
// given service; available only in test binaries.
func NewFirewallRuleResourceWithService(svc firewallServiceIface) resource.Resource {
	return &firewallRuleResource{svc: svc}
}
