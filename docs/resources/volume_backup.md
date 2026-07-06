---
page_title: "zcp_volume_backup Resource"
description: |-
  Create and manage scheduled backups for ZCP block storage volumes.
---

# zcp_volume_backup

Manages a scheduled backup for a ZCP block storage volume. The API has no update endpoint for backup schedules, so every change forces replacement.

## Example Usage

```terraform
resource "zcp_volume_backup" "data_daily" {
  volume         = zcp_volume.data.id
  interval       = "dailyAt"
  at             = 1
  plan           = "backup-yow"
  billing_cycle  = "hourly"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
}
```

## Import

Import using `<slug>/<region>/<cloud_provider>/<volume>/<interval>/<billing_cycle>[/<plan>/<project>]`:

```shell
terraform import zcp_volume_backup.data_daily root-backup-b1/yow-1/zsoftly/data-1234/dailyAt/hourly
```

## Schema

### Required

- `volume` (String) Slug of the block storage volume to back up. Changing this forces replacement.
- `interval` (String) Backup interval (e.g. `dailyAt`). Changing this forces replacement.
- `billing_cycle` (String) Billing cycle (e.g. `hourly`, `monthly`). Changing this forces replacement.
- `cloud_provider` (String) Cloud provider slug. Changing this forces replacement.
- `region` (String) Region slug (e.g. `yow-1`). Changing this forces replacement.

### Optional

- `at` (Number) Hour at which the backup triggers (e.g. `1` for 1 AM). Defaults to `1`. Changing this forces replacement.
- `immediate` (Boolean) Run a backup immediately after creating the schedule. Changing this forces replacement.
- `plan` (String) Backup plan slug (e.g. `backup-yow`). Changing this forces replacement.
- `pseudo_service` (String) Pseudo-service slug the backup bills under. Defaults to `Virtual Machine Backup`. Changing this forces replacement.
- `project` (String) Project slug. Inherits from the provider `default_project` if omitted. Changing this forces replacement.

### Read-Only

- `id` (String) Backup slug.
