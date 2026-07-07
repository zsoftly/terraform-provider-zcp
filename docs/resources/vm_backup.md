---
page_title: "zcp_vm_backup Resource"
description: |-
  Create and manage scheduled backups for ZCP instances.
---

# zcp_vm_backup

Manages a scheduled backup for a ZCP instance. The API has no update endpoint
for backup schedules, so every change forces replacement.

## Example Usage

```terraform
resource "zcp_vm_backup" "web_daily" {
  virtual_machine = zcp_instance.web.id
  interval        = "daily"
  at              = 2
  plan            = "backup-yow"
  billing_cycle   = "hourly"
  cloud_provider  = data.zcp_region.yow.cloud_provider
  region          = data.zcp_region.yow.slug
}
```

## Import

Import using
`<slug>/<region>/<cloud_provider>/<virtual_machine>/<interval>/<plan>/<billing_cycle>[/<project>]`:

```shell
terraform import zcp_vm_backup.web_daily vm1-backup-b1/yow-1/zsoftly/vm1-abc/daily/backup-yow/hourly
```

## Schema

### Required

- `virtual_machine` (String) Slug of the instance to back up. Changing this
  forces replacement.
- `interval` (String) Backup interval (e.g. `daily`, `weekly`). Changing this
  forces replacement.
- `plan` (String) Backup plan slug (e.g. `backup-yow`). Changing this forces
  replacement.
- `billing_cycle` (String) Billing cycle (e.g. `hourly`, `monthly`). Changing
  this forces replacement.
- `cloud_provider` (String) Cloud provider slug. Changing this forces
  replacement.
- `region` (String) Region slug (e.g. `yow-1`). Changing this forces
  replacement.

### Optional

- `at` (Number) Hour of day for the scheduled backup (0-23). Defaults to `0`.
  Changing this forces replacement.
- `immediate` (Boolean) Run a backup immediately after creating the schedule.
  Changing this forces replacement.
- `pseudo_service` (String) Pseudo-service slug the backup bills under. Defaults
  to `vm-backup`. Changing this forces replacement.
- `project` (String) Project slug. Inherits from the provider `default_project`
  if omitted. Changing this forces replacement.
- `timeouts` (Block) Configurable `create` and `delete` timeouts.

### Read-Only

- `id` (String) VM backup slug.
- `state` (String) Current state of the backup.
