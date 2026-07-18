# Changelog

This file documents all notable changes to the ZCP provider for Terraform and
OpenTofu. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project adheres
to [Semantic Versioning](https://semver.org/).

## [v0.1.1] - 2026-07-18

### Changed

- Upgraded the zcp-cli SDK from v0.0.23 to v0.0.24. Public IP and load balancer
  listings now page through every result instead of stopping at the first page.

### Fixed

- `zcp_instance` destroy now releases the VM's auto-assigned public IP. It goes
  through the service-cancellation workflow that the CMP Web UI runs, which
  frees the IP as part of the deletion, so destroy no longer leaves a billable
  address. A public IP bound through `zcp_ip_address`/`zcp_ip_association` is
  left to its own resource. Set `assign_public_ip = false` to create the
  instance without an auto-assigned public IP. Verified end to end with
  Terraform and OpenTofu.
- `zcp_load_balancer` destroy now deletes the load balancer through the
  service-cancellation workflow the CMP Web UI runs, instead of the direct
  delete endpoint, which could return success without removing the balancer.
  Destroy waits for the balancer to be gone and re-issues the cancellation if
  the platform stalls it, which it can do when the balancer's cancellation runs
  at the same time as another service's teardown. Destroy then releases the
  public IP the balancer acquired (`acquire_new_ip = true`, the default), which
  was previously orphaned. The release is best-effort: a warning is issued if it
  does not complete. A network source-NAT IP is never released, since the
  network owns it and frees it when the network is destroyed. A public IP bound
  through `ip_address` is left to its own resource.

## [v0.1.0] - 2026-07-07

Initial release. Built on the zcp-cli SDK v0.0.23 and verified against the live
ZCP platform.

### Added

38 resources:

- Compute: `zcp_instance`, `zcp_volume`, `zcp_volume_snapshot`,
  `zcp_volume_backup`, `zcp_vm_snapshot`, `zcp_vm_backup`, `zcp_affinity_group`,
  `zcp_iso`, `zcp_account_template`, `zcp_ssh_key`
- Networking: `zcp_vpc`, `zcp_network`, `zcp_network_acl`,
  `zcp_network_acl_rule`, `zcp_firewall_rule`, `zcp_egress_rule`,
  `zcp_port_forward`, `zcp_ip_address`, `zcp_ip_association`
- Load balancing: `zcp_load_balancer`, `zcp_load_balancer_rule`,
  `zcp_load_balancer_attachment`
- Autoscaling: `zcp_autoscale_group`, `zcp_autoscale_policy`,
  `zcp_autoscale_condition`
- VPN: `zcp_vpc_vpn_gateway`, `zcp_vpn_customer_gateway`, `zcp_vpn_user`,
  `zcp_remote_access_vpn`
- Kubernetes: `zcp_kubernetes_cluster` with in-place version upgrade and plan
  scaling
- DNS: `zcp_dns_domain`, `zcp_dns_record` (PowerDNS record sets addressed by
  name and type)
- Object storage: `zcp_object_storage`, `zcp_object_storage_bucket`
- Account and governance: `zcp_project`, `zcp_sub_user`, `zcp_role`,
  `zcp_budget_alert`

12 data sources: `zcp_billing_cycle`, `zcp_instance`, `zcp_kubernetes_version`,
`zcp_network`, `zcp_permissions`, `zcp_plan`, `zcp_project`, `zcp_region`,
`zcp_ssh_key`, `zcp_storage_category`, `zcp_template`, `zcp_vpc`

Provider features:

- Configuration via `bearer_token`, `api_url`, and `default_project`, each with
  a `ZCP_*` environment variable fallback
- Import on every resource, with composite import IDs documented per resource
- Create and delete polling with configurable `timeouts` blocks
- Terraform plugin protocol 6 (Terraform >= 1.0, OpenTofu >= 1.6)
- Documentation for every resource and data source under `docs/`, runnable
  examples under `examples/`
