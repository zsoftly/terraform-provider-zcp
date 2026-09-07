# Changelog

This file documents all notable changes to the ZCP provider for Terraform and
OpenTofu. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project adheres
to [Semantic Versioning](https://semver.org/).

## [v0.1.4] - 2026-09-07

### Added

- **`zcp_instance` supports the `Vpc` network type and attaching multiple
  networks.** This brings the resource up to the zcp-cli v0.0.27 instance-create
  capabilities. `network_type` selects `Isolated` (the default when omitted),
  `L2`, or `Vpc`. `vr_plan` auto-creates a virtual router for `Vpc` instances.
  `networks` attaches a list of existing networks and supersedes `network`.
  `default_network` picks the default among more than one entry in `networks`
  and must also match a single `network` when one is set instead. `network_type`
  is a plain optional attribute, not computed, so an omitted value is never
  written to state and upgrading from a provider version that predates it does
  not plan a replacement for any existing instance. `ValidateConfig` enforces
  the same combinations the CLI enforces: `vr_plan` only for `Vpc`,
  `network_plan` never for `Vpc`, one network source required (an empty
  `networks = []` counts as none), and `default_network` required and valid when
  `networks` has more than one entry. A not-allowed attribute for the configured
  `network_type` is now reported on its own, instead of alongside a redundant
  missing-network-source error. A `networks` entry that is not yet known at plan
  time, such as one created in the same apply, no longer raises a
  value-conversion error, and neither does an unresolved `default_network`.
  `public_ip` now prefers the instance's `ipaddresses` entry marked as the
  public IP, matching `data.zcp_instance`, before falling back to the top-level
  field and then the account IP list. Import now rejects an ID with more
  segments than the documented format instead of folding the extra segments into
  `networks`. An unknown `network`, `network_plan`, `vr_plan`, or `networks`
  value at plan time, such as `network = zcp_network.app.id` before that
  resource exists, no longer triggers a false "missing network configuration"
  error. `network_type = "L2"` with `assign_public_ip` true or omitted (it
  defaults to true) is now rejected, matching the CLI, since the platform
  rejects a public IP on an L2 network. Existing `zcp_instance` resources plan
  no changes after upgrading, and `terraform import` IDs may omit the new
  trailing segments.
- **New `zcp_volume` data source.** Look up an existing block storage volume by
  slug, with optional `region` and `project` scoping. It exposes `name`, `size`,
  `volume_type`, `instance_id`, and `created_at` (relates to
  zsoftly/zcp-cli#59).
- **`data.zcp_instance` exposes `root_volume` and `volumes`.** `root_volume` is
  the slug of the instance's root disk and `volumes` lists the slugs of attached
  volumes, root first. The lookup retrieves every result page within the
  configured region and project scope. Point `zcp_volume_backup` at
  `data.zcp_instance.web.root_volume` instead of hardcoding a volume slug that
  changes every time the instance is rebuilt. `data.zcp_instance` also gained
  optional `region` and `project` inputs to scope this lookup, and `public_ip`
  now reads the address from the instance's IP list when the top-level field is
  null, matching the platform's actual behavior (relates to zsoftly/zcp-cli#59).

### Changed

- **Upgraded the zcp-cli SDK from v0.0.28 to v0.0.29.** Volume listings now
  retrieve every API page, so `data.zcp_volume` lookups and the volume
  collection behind `data.zcp_instance.root_volume` and `.volumes` no longer
  miss results beyond the first page within their configured scope.
- **Upgraded the zcp-cli SDK from v0.0.26 to v0.0.28.** The Go toolchain moves
  from 1.26.6 to 1.26.8. The `go` directive moves to 1.26.0 because the SDK
  requires it.
- **`zcp_vm_backup` binds the created schedule by VM slug.** Create used to diff
  the backup list against a pre-create baseline and adopt whatever new entry
  appeared. It now matches the entry whose VM slug equals the configured
  `virtual_machine`. The baseline diff stays as a secondary guard against a
  backup that already existed for that VM.
- **`zcp_vm_backup` documents `state` as always null.** The VM backup listing
  never returns a state field on this platform. The attribute stays for
  compatibility but never holds a value.
- **Standardized on `stringvalidator.OneOf` for enum-style attributes.**
  `zcp_network_acl_rule`'s `action`, `traffic_type`, and `protocol` validators
  now use the `terraform-plugin-framework-validators` package, matching
  `zcp_vm_backup` and `zcp_volume_backup`. The hand-rolled
  `stringOneOfValidator` is removed.

### Fixed

- **`zcp_volume_backup` reads again.** The `blockstorages/backups` listing
  returns `at` as a string and nests the volume under `blockstorage`. The
  provider's read failed to decode this shape. The zcp-cli SDK upgrade to
  v0.0.28 fixes the decode (fixes zsoftly/zcp-cli#65).
- **`zcp_vm_backup` can now be destroyed.** The API rejects a direct `DELETE` on
  a backup schedule. Delete now submits a cancellation request through the
  billing service-cancellation endpoint, the same workflow the CMP web UI uses,
  and waits for the schedule to disappear from the list. A slug that no longer
  exists is treated as already deleted (fixes zsoftly/zcp-cli#58). Destroy also
  retries recognized transient list errors while waiting for the schedule to
  disappear. A permanent error stops the destroy immediately. If the schedule
  does not disappear before the deadline, the provider surfaces the last
  transient error.
- **`interval` is validated on `zcp_vm_backup` and `zcp_volume_backup`, and the
  docs are corrected.** The API accepts only `dailyAt` and `hourlyAt` and
  rejects `daily`, `weekly`, `monthly`, and `hourly`. The docs previously showed
  `daily` and `weekly` as valid values. Both resources now reject unsupported
  values before apply and the docs list the accepted set (fixes #15).
- **`zcp_firewall_rule` now fails fast on a VPC public IP it can identify as
  such, instead of timing out.** The API accepts a firewall rule create request
  on a public IP that belongs to a VPC but never applies it. Create used to poll
  until timeout in that case. The provider now checks the account IP list before
  create and returns an error naming `zcp_network_acl` and
  `zcp_network_acl_rule` as the VPC alternative when the IP is found in that
  list and belongs to a VPC. The check is best effort: if the IP cannot be found
  in the list (or the list call fails), create proceeds and can still time out
  (fixes #16).
- **`zcp_ip_address` now explains the VPC-without-a-network ordering problem.**
  Allocating a public IP into a VPC that has no network tier fails on the API
  with a 422 error. The provider now returns a clear error naming the fix:
  create the tier first and add `depends_on` on it. Docs show the pattern
  (relates to #14).
- **`zcp_network_acl` documentation corrected from stateless to stateful.**
  Platform ACLs track connections and accept replies to allowed traffic
  automatically. Ingress and egress rules do not correlate. Inbound listeners
  still need explicit ingress rules (fixes #13).

## [v0.1.3] - 2026-07-20

### Fixed

- **`zcp_port_forward` and `zcp_firewall_rule` now capture the created rule's
  ID.** The create endpoint accepts the request asynchronously and returns no
  rule object, so the resources were storing an empty ID. A read then failed to
  find the rule and Terraform planned to recreate it on every apply. Both
  resources now poll the rule list after create and match on protocol and ports
  (and CIDR for firewall rules) to record the real ID and state. Blank plan
  fields do not narrow the match, ports compare with `0` treated as no port, and
  CIDR lists compare as unordered sets so the API can echo them in any order.
  Known limitation: when an identical rule already exists on the same IP, the
  first list match wins, because the create API returns no correlation token.

### Changed

- Upgraded the zcp-cli SDK from v0.0.25 to v0.0.26. This corrects how port
  forwarding rule ports are decoded from the API, which the port-forward
  resource relies on to match a rule after create.

### Security

- Upgraded `golang.org/x/text` to v0.39.0 to resolve GO-2026-5970, an infinite
  loop on invalid input. The vulnerable path was reachable through the load
  balancer resource. `golang.org/x/sys` and `golang.org/x/tools` moved forward
  with it.

## [v0.1.2] - 2026-07-18

### Added

- `zcp_dns_record` now supports `MX` records through a new `priority` argument
  (0-65535). Put the mail server in `content` and the preference number in
  `priority`. Priority is required for `MX` and rejected for every other type,
  both checked at plan time. The previous SDK never sent priority, so `MX`
  records failed with an API error. Verified against the live platform.

### Changed

- Upgraded the zcp-cli SDK from v0.0.24 to v0.0.25, which adds DNS record
  priority support.
- `zcp_dns_record` no longer lists `SRV` among the documented `type` values. The
  DNS API rejects `SRV` and `LOC` records, so advertising `SRV` was misleading.
  This is a documentation change. `type` is still a free-form string.

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
  was previously orphaned. If that release fails, destroy now fails with an
  error naming the IP instead of leaving it allocated silently; an
  already-released IP is treated as success. A network source-NAT IP is never
  released, since the network owns it and frees it when the network is
  destroyed. A public IP bound through `ip_address` is left to its own resource.

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
