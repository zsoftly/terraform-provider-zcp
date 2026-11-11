# terraform-provider-zcp v1.3.1 Release Notes

Provider v1.3.1 fixes `zcp_network_acl` for VPCs with more than one page of
network ACLs and builds against `github.com/zsoftly/zcp-cli` v1.3.1.

## Fixed

**ACLs in larger VPCs stay in state.** The ZCP API returns a VPC's network ACLs
in pages, 10 by default. The provider read only the first page. Once a VPC had
more than 10 ACLs, refresh treated the oldest ones as deleted. The next plan
then proposed to recreate those ACLs and replace all of their rules. Applying it
would interrupt traffic on the attached networks. The provider did not delete
the ACLs or their rules on the platform.

The provider now reads every page when it refreshes `zcp_network_acl` and when
it looks up the ID of a newly created ACL.

If you held back applies because a plan proposed to recreate existing ACLs,
upgrade first, then run a new plan. Refresh keeps those ACLs, and the plan drops
their create and destroy actions.

## Upgrade

The `~> 1.3.0` constraint already allows this release. Run
`terraform init -upgrade` for Terraform or `tofu init -upgrade` for OpenTofu,
then review the plan before applying changes.

Reported in [#36](https://github.com/zsoftly/terraform-provider-zcp/issues/36).
