// export_test.go exposes package-internal constructors for use in
// external test packages. This file is only compiled during test builds.
package provider

import (
	"time"

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

// NewStorageCategoryDataSourceWithLister creates a storageCategoryDataSource
// pre-wired with the given lister; available only in test binaries.
func NewStorageCategoryDataSourceWithLister(l storageCategoryLister) datasource.DataSource {
	return &storageCategoryDataSource{svc: l}
}

// NewBillingCycleDataSourceWithLister creates a billingCycleDataSource
// pre-wired with the given lister; available only in test binaries.
func NewBillingCycleDataSourceWithLister(l billingCycleLister) datasource.DataSource {
	return &billingCycleDataSource{svc: l}
}

// NewNetworkDataSourceWithLister creates a networkDataSource pre-wired with
// the given lister; available only in test binaries.
func NewNetworkDataSourceWithLister(l networkLister) datasource.DataSource {
	return &networkDataSource{svc: l}
}

// NewVPCDataSourceWithLister creates a vpcDataSource pre-wired with the
// given lister; available only in test binaries.
func NewVPCDataSourceWithLister(l vpcLister) datasource.DataSource {
	return &vpcDataSource{svc: l}
}

// NewSSHKeyDataSourceWithLister creates an sshKeyDataSource pre-wired with
// the given lister; available only in test binaries.
func NewSSHKeyDataSourceWithLister(l sshKeyLister) datasource.DataSource {
	return &sshKeyDataSource{svc: l}
}

// NewKubernetesVersionDataSourceWithLister creates a kubernetesVersionDataSource
// pre-wired with the given lister; available only in test binaries.
func NewKubernetesVersionDataSourceWithLister(l kubernetesVersionLister) datasource.DataSource {
	return &kubernetesVersionDataSource{svc: l}
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

// NewFirewallRuleResourceWithServices additionally wires a public-IP lister,
// used to reject firewall rules on VPC public IPs; available only in test
// binaries.
func NewFirewallRuleResourceWithServices(svc firewallServiceIface, ipSvc publicIPLister) resource.Resource {
	return &firewallRuleResource{svc: svc, ipSvc: ipSvc}
}

// NewFirewallRuleResourceWithServicesAndProject additionally wires a
// provider-level default project, used to scope the VPC public-IP guard's
// list call; available only in test binaries.
func NewFirewallRuleResourceWithServicesAndProject(svc firewallServiceIface, ipSvc publicIPLister, defaultProject string) resource.Resource {
	return &firewallRuleResource{svc: svc, ipSvc: ipSvc, defaultProject: defaultProject}
}

// Test binaries shorten the post-Running private-IP wait so create tests with
// fixtures that never report an address finish in milliseconds, not minutes.
func init() {
	privateIPPollInterval = 20 * time.Millisecond
	privateIPPollWindow = 100 * time.Millisecond
}

// NewInstanceResourceWithService creates an instanceResource pre-wired with the
// given service; available only in test binaries.
func NewInstanceResourceWithService(svc instanceServiceIface) resource.Resource {
	return &instanceResource{svc: svc}
}

// NewInstanceResourceWithServices additionally wires a public-IP lister;
// available only in test binaries.
func NewInstanceResourceWithServices(svc instanceServiceIface, ipSvc publicIPLister) resource.Resource {
	return &instanceResource{svc: svc, ipSvc: ipSvc}
}

// NewVolumeResourceWithService creates a volumeResource pre-wired with the
// given service; available only in test binaries.
func NewVolumeResourceWithService(svc volumeServiceIface) resource.Resource {
	return &volumeResource{svc: svc}
}

// NewKubernetesClusterResourceWithService creates a kubernetesClusterResource
// pre-wired with the given service; available only in test binaries.
func NewKubernetesClusterResourceWithService(svc kubernetesServiceIface) resource.Resource {
	return &kubernetesClusterResource{svc: svc}
}

// SetKubernetesPollIntervalForTest overrides Kubernetes polling intervals and
// returns a restore function.
func SetKubernetesPollIntervalForTest(d time.Duration) func() {
	prev := kubernetesPollInterval
	kubernetesPollInterval = d
	return func() {
		kubernetesPollInterval = prev
	}
}

// NewNetworkACLResourceWithService creates a networkACLResource pre-wired with
// the given service; available only in test binaries.
func NewNetworkACLResourceWithService(svc aclServiceIface) resource.Resource {
	return &networkACLResource{svc: svc}
}

// NewNetworkACLRuleResourceWithService creates a networkACLRuleResource pre-wired
// with the given service; available only in test binaries.
func NewNetworkACLRuleResourceWithService(svc aclServiceIface) resource.Resource {
	return &networkACLRuleResource{svc: svc}
}

// NewNetworkResourceWithServices creates a networkResource pre-wired with the
// given network and ACL services; available only in test binaries.
func NewNetworkResourceWithServices(svc networkServiceIface, aclSvc aclServiceIface) resource.Resource {
	return &networkResource{svc: svc, aclSvc: aclSvc}
}

// NewIPAssociationResourceWithService creates an ipAssociationResource pre-wired
// with the given service; available only in test binaries.
func NewIPAssociationResourceWithService(svc ipAssociationServiceIface) resource.Resource {
	return &ipAssociationResource{svc: svc}
}

// NewEgressRuleResourceWithService creates an egressRuleResource pre-wired with
// the given service; available only in test binaries.
func NewEgressRuleResourceWithService(svc egressServiceIface) resource.Resource {
	return &egressRuleResource{svc: svc}
}

// NewAffinityGroupResourceWithService creates an affinityGroupResource pre-wired
// with the given service; available only in test binaries.
func NewAffinityGroupResourceWithService(svc affinityGroupServiceIface) resource.Resource {
	return &affinityGroupResource{svc: svc}
}

// NewDNSDomainResourceWithService creates a dnsDomainResource pre-wired with
// the given service; available only in test binaries.
func NewDNSDomainResourceWithService(svc dnsServiceIface) resource.Resource {
	return &dnsDomainResource{svc: svc}
}

// NewDNSRecordResourceWithService creates a dnsRecordResource pre-wired with
// the given service and deleter; available only in test binaries.
func NewDNSRecordResourceWithService(svc dnsServiceIface, deleter dnsRecordDeleter) resource.Resource {
	return &dnsRecordResource{svc: svc, deleter: deleter}
}

// NewLoadBalancerResourceWithService creates a loadBalancerResource pre-wired
// with the given service; available only in test binaries.
func NewLoadBalancerResourceWithServices(svc loadBalancerServiceIface, ipSvc lbIPServiceIface) resource.Resource {
	return &loadBalancerResource{svc: svc, ipSvc: ipSvc, deletePollInterval: time.Millisecond}
}

func NewLoadBalancerResourceWithService(svc loadBalancerServiceIface) resource.Resource {
	return &loadBalancerResource{svc: svc, deletePollInterval: time.Millisecond}
}

// NewLoadBalancerRuleResourceWithService creates a loadBalancerRuleResource
// pre-wired with the given service; available only in test binaries.
func NewLoadBalancerRuleResourceWithService(svc loadBalancerServiceIface) resource.Resource {
	return &loadBalancerRuleResource{svc: svc}
}

// NewLoadBalancerAttachmentResourceWithService creates a loadBalancerAttachmentResource
// pre-wired with the given service; available only in test binaries.
func NewLoadBalancerAttachmentResourceWithService(svc loadBalancerServiceIface) resource.Resource {
	return &loadBalancerAttachmentResource{svc: svc}
}

// NewLoadBalancerAttachmentResourceWithServiceAndProject additionally wires a
// provider-level default project; available only in test binaries.
func NewLoadBalancerAttachmentResourceWithServiceAndProject(svc loadBalancerServiceIface, defaultProject string) resource.Resource {
	return &loadBalancerAttachmentResource{svc: svc, defaultProject: defaultProject}
}

// NewObjectStorageResourceWithService creates an objectStorageResource pre-wired
// with the given service; available only in test binaries.
func NewObjectStorageResourceWithService(svc objectStorageServiceIface) resource.Resource {
	return &objectStorageResource{svc: svc}
}

// NewObjectStorageBucketResourceWithService creates an objectStorageBucketResource
// pre-wired with the given service; available only in test binaries.
func NewObjectStorageBucketResourceWithService(svc objectStorageServiceIface) resource.Resource {
	return &objectStorageBucketResource{svc: svc}
}

// NewVMSnapshotResourceWithService creates a vmSnapshotResource pre-wired with
// the given service; available only in test binaries.
func NewVMSnapshotResourceWithService(svc vmSnapshotServiceIface) resource.Resource {
	return &vmSnapshotResource{svc: svc}
}

// NewVMBackupResourceWithService creates a vmBackupResource pre-wired with
// the given service; available only in test binaries. The destroy poll
// interval is shortened so delete tests that retry past a transient error
// finish quickly.
func NewVMBackupResourceWithService(svc vmBackupServiceIface) resource.Resource {
	return &vmBackupResource{svc: svc, deletePollInterval: time.Millisecond}
}

// NewVolumeSnapshotResourceWithService creates a volumeSnapshotResource pre-wired
// with the given service; available only in test binaries.
func NewVolumeSnapshotResourceWithService(svc volumeSnapshotServiceIface) resource.Resource {
	return &volumeSnapshotResource{svc: svc}
}

// NewVolumeBackupResourceWithService creates a volumeBackupResource pre-wired
// with the given service; available only in test binaries. The destroy poll
// interval is shortened so delete tests that retry past a transient error
// finish quickly.
func NewVolumeBackupResourceWithService(svc volumeBackupServiceIface) resource.Resource {
	return &volumeBackupResource{svc: svc, deletePollInterval: time.Millisecond}
}

// NewAutoscaleGroupResourceWithService creates an autoscaleGroupResource
// pre-wired with the given service; available only in test binaries.
func NewAutoscaleGroupResourceWithService(svc autoscaleServiceIface) resource.Resource {
	return &autoscaleGroupResource{svc: svc}
}

// NewAutoscalePolicyResourceWithService creates a policy-kind autoscaleRuleResource
// pre-wired with the given service; available only in test binaries.
func NewAutoscalePolicyResourceWithService(svc autoscaleServiceIface) resource.Resource {
	return &autoscaleRuleResource{svc: svc, kind: autoscalePolicyKind}
}

// NewAutoscaleConditionResourceWithService creates a condition-kind autoscaleRuleResource
// pre-wired with the given service; available only in test binaries.
func NewAutoscaleConditionResourceWithService(svc autoscaleServiceIface) resource.Resource {
	return &autoscaleRuleResource{svc: svc, kind: autoscaleConditionKind}
}

// NewISOResourceWithService creates an isoResource pre-wired with the given
// service; available only in test binaries.
func NewISOResourceWithService(svc isoServiceIface) resource.Resource {
	return &isoResource{svc: svc}
}

// NewAccountTemplateResourceWithService creates an accountTemplateResource
// pre-wired with the given service; available only in test binaries.
func NewAccountTemplateResourceWithService(svc accountTemplateServiceIface) resource.Resource {
	return &accountTemplateResource{svc: svc}
}

// NewProjectResourceWithService creates a projectResource pre-wired with the
// given service; available only in test binaries.
func NewProjectResourceWithService(svc projectServiceIface) resource.Resource {
	return &projectResource{svc: svc}
}

// NewSubUserResourceWithService creates a subUserResource pre-wired with the
// given service; available only in test binaries.
func NewSubUserResourceWithService(svc subUserServiceIface) resource.Resource {
	return &subUserResource{svc: svc}
}

// NewRoleResourceWithService creates a roleResource pre-wired with the given
// service; available only in test binaries.
func NewRoleResourceWithService(svc roleServiceIface) resource.Resource {
	return &roleResource{svc: svc}
}

// NewBudgetAlertResourceWithService creates a budgetAlertResource pre-wired
// with the given service; available only in test binaries.
func NewBudgetAlertResourceWithService(svc budgetAlertServiceIface) resource.Resource {
	return &budgetAlertResource{svc: svc}
}

// NewRemoteAccessVPNResourceWithService creates a remoteAccessVPNResource
// pre-wired with the given service; available only in test binaries.
func NewRemoteAccessVPNResourceWithService(svc remoteAccessVPNServiceIface) resource.Resource {
	return &remoteAccessVPNResource{svc: svc}
}

// NewInstanceDataSourceWithGetter creates an instanceDataSource pre-wired with
// the given getter; available only in test binaries.
func NewInstanceDataSourceWithGetter(g instanceGetter) datasource.DataSource {
	return &instanceDataSource{svc: g}
}

// NewInstanceDataSourceWithServices creates an instanceDataSource pre-wired
// with the given getter and volume lister; available only in test binaries.
func NewInstanceDataSourceWithServices(g instanceGetter, v volumeLister) datasource.DataSource {
	return &instanceDataSource{svc: g, volSvc: v}
}

// NewVolumeDataSourceWithLister creates a volumeDataSource pre-wired with the
// given lister; available only in test binaries.
func NewVolumeDataSourceWithLister(l volumeLister) datasource.DataSource {
	return &volumeDataSource{svc: l}
}

// NewPermissionsDataSourceWithLister creates a permissionsDataSource pre-wired
// with the given lister; available only in test binaries.
func NewPermissionsDataSourceWithLister(l permissionLister) datasource.DataSource {
	return &permissionsDataSource{svc: l}
}
