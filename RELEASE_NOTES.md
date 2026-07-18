# terraform-provider-zcp Release Notes

## v0.1.1 (2026-07-18)

A bug-fix release. Destroying a resource now releases the public IP it created,
instead of leaving it allocated.

- `zcp_instance` destroy releases the VM's auto-assigned public IP through the
  service-cancellation workflow. Set `assign_public_ip = false` to create the
  instance without an auto-assigned public IP.
- `zcp_load_balancer` destroy deletes the load balancer through the
  service-cancellation workflow instead of the direct delete endpoint, which
  could report success without removing it. Destroy waits for the balancer to be
  gone and re-issues the cancellation if the platform stalls it (which it can do
  when the balancer's cancellation runs at the same time as another service's
  teardown). It then releases the public IP the balancer acquired
  (`acquire_new_ip = true`, the default) on a best-effort basis, with a warning
  if the release does not complete. A network source-NAT IP is never released,
  since the network owns it, and a bound `ip_address` is left to its own
  resource.

Built on the zcp-cli SDK v0.0.24. Verified end to end with Terraform and
OpenTofu.

---

## v0.1.0 (2026-07-07)

First release of the ZCP provider for Terraform and OpenTofu. It manages the
ZSoftly Cloud Platform with 38 resources and 12 data sources, covering
everything the `zcp` CLI supports: instances, volumes, snapshots and backups,
VPCs and networks, firewall and egress rules, load balancers, autoscaling,
Kubernetes clusters, DNS, object storage, VPN, and account governance.

Built on the zcp-cli SDK v0.0.23. Every resource supports import. We verified
the core paths against the live platform (create, read, update, delete, and
zero-diff re-plan).

Highlights:

- **Full platform coverage.** One provider block manages compute, networking,
  Kubernetes, DNS, storage, and governance. See the [CHANGELOG](CHANGELOG.md)
  for the complete resource list.
- **DNS records as record sets.** `zcp_dns_record` matches how the PowerDNS
  backend works: one resource per name and type pair, no fake record IDs.
- **In-place Kubernetes changes.** Version upgrades and plan scaling update the
  cluster instead of replacing it.
- **DNS defaults.** `zcp_dns_domain` defaults `cloud_provider` to `dns` and
  `region` to `default`, the only valid combination on the platform.
- **Plugin protocol 6.** Works with Terraform 1.0 and later and OpenTofu 1.6 and
  later.

---

## Installation and upgrade

Add the provider to your configuration and run `terraform init` or `tofu init`:

```hcl
terraform {
  required_providers {
    zcp = {
      source  = "zsoftly/zcp"
      version = "~> 0.1"
    }
  }
}

provider "zcp" {
  # bearer_token can also come from the ZCP_BEARER_TOKEN environment variable
  default_project = "default-9"
}
```

**Authentication:** set `ZCP_BEARER_TOKEN` in the environment or `bearer_token`
in the provider block. Generate the token in the
[ZCP console](https://cloud.zcp.zsoftly.ca) under **Account → API Keys**.
`ZCP_API_URL` and `ZCP_PROJECT` override the API endpoint and default project.

**Manual install:** download the zip for your platform from the
[Releases](https://github.com/zsoftly/terraform-provider-zcp/releases) page and
unpack it into your plugin directory, for example
`~/.terraform.d/plugins/registry.terraform.io/zsoftly/zcp/0.1.0/<os>_<arch>/`.

**Verify:**

```bash
terraform init
terraform providers   # shows provider registry address and version
```

---

## What is included

38 resources and 12 data sources. The groups:

| Area           | Resources                                                                                                                |
| -------------- | ------------------------------------------------------------------------------------------------------------------------ |
| Compute        | instance, volume, volume_snapshot, volume_backup, vm_snapshot, vm_backup, affinity_group, iso, account_template, ssh_key |
| Networking     | vpc, network, network_acl, network_acl_rule, firewall_rule, egress_rule, port_forward, ip_address, ip_association        |
| Load balancing | load_balancer, load_balancer_rule, load_balancer_attachment                                                              |
| Autoscaling    | autoscale_group, autoscale_policy, autoscale_condition                                                                   |
| VPN            | vpc_vpn_gateway, vpn_customer_gateway, vpn_user, remote_access_vpn                                                       |
| Kubernetes     | kubernetes_cluster                                                                                                       |
| DNS            | dns_domain, dns_record                                                                                                   |
| Object storage | object_storage, object_storage_bucket                                                                                    |
| Governance     | project, sub_user, role, budget_alert                                                                                    |

Data sources: billing_cycle, instance, kubernetes_version, network, permissions,
plan, project, region, ssh_key, storage_category, template, vpc.

Each resource has a documentation page under `docs/` with its argument
reference, import format, and a runnable example under `examples/`.

## Known platform behaviors

- Deleting an instance releases its auto-assigned public IP through the
  service-cancellation workflow, so destroy does not leave a billable address. A
  public IP bound through `zcp_ip_address`/`zcp_ip_association` is left to its
  own resource.
- The platform's cached instance state lags behind reality, sometimes by many
  minutes. The provider polls the live `/meta` endpoint, which reconciles
  against the hypervisor, so creates and resizes finish as soon as the VM is up.
- On some networks, egress rule creation returns success while the backend
  creates nothing. The provider polls the rule list and fails loudly instead of
  recording a nonexistent rule.
- The DNS backend appends the zone to record names. `zcp_dns_record` takes
  relative names (`www`, not `www.example.com`).
- `content` on `zcp_dns_record` and `public_key` on `zcp_ssh_key` are write-only
  because the API does not return them in a comparable form. The provider does
  not detect out-of-band changes to those fields.
