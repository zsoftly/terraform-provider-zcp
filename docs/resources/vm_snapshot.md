---
page_title: "zcp_vm_snapshot Resource"
description: |-
  Create and manage ZCP instance snapshots.
---

# zcp_vm_snapshot

Manages a point-in-time snapshot of a ZCP instance. Snapshots are immutable, so
every change forces replacement. Reverting an instance to a snapshot is an
operational action outside Terraform (`zcp vm-snapshot revert`).

## Example Usage

```terraform
resource "zcp_vm_snapshot" "pre_upgrade" {
  virtual_machine = zcp_instance.web.id
  name            = "pre-upgrade"
  billing_cycle   = "monthly"
  plan            = "vm-snapshot-yow"
  cloud_provider  = data.zcp_region.yow.cloud_provider
  region          = data.zcp_region.yow.slug
}
```

## Import

Import using
`<slug>/<region>/<cloud_provider>/<virtual_machine>/<billing_cycle>[/<plan>/<project>]`:

```shell
terraform import zcp_vm_snapshot.pre_upgrade pre-upgrade-s1/yow-1/zsoftly/vm1-abc/monthly
```

## Schema

### Required

- `virtual_machine` (String) Slug of the instance to snapshot. Changing this
  forces replacement.
- `name` (String) Snapshot name. Changing this forces replacement.
- `billing_cycle` (String) Billing cycle (e.g. `hourly`, `monthly`). Changing
  this forces replacement.
- `cloud_provider` (String) Cloud provider slug. Changing this forces
  replacement.
- `region` (String) Region slug (e.g. `yow-1`). Changing this forces
  replacement.

### Optional

- `plan` (String) Snapshot plan slug (e.g. `vm-snapshot-yow`). Changing this
  forces replacement.
- `service` (String) Service slug the snapshot bills under (e.g.
  `virtual-machine`). Changing this forces replacement.
- `is_memory` (Boolean) Include memory state in the snapshot. Changing this
  forces replacement.
- `project` (String) Project slug. Inherits from the provider `default_project`
  if omitted. Changing this forces replacement.
- `timeouts` (Block) Configurable `create` and `delete` timeouts.

### Read-Only

- `id` (String) VM snapshot slug.
- `state` (String) Current state of the snapshot.
