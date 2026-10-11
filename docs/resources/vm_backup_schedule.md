---
page_title: "zcp_vm_backup_schedule Resource"
description: |-
  Create and manage VM backup schedules.
---

# zcp_vm_backup_schedule

Manages a VM backup schedule. Schedule updates change its timing, timezone, and
retention. Set `paused` to pause or resume a schedule. Use the ZCP CLI for
one-off runs and restores because Terraform does not model them as durable
state. The provider intentionally rejects `everyOtherDay` before writing because
the platform persists it and then returns an error.

## Example Usage

```terraform
resource "zcp_vm_backup_schedule" "web" {
  virtual_machine = zcp_instance.web.id
  interval        = "weeklyOn"
  day             = 0
  at              = "02:00"
  timezone        = "America/Toronto"
  retention       = 7
  region          = data.zcp_region.yow.slug
}
```

## Import

Import using `<id>/<region>[/<project>]`.

```shell
terraform import zcp_vm_backup_schedule.web policy-1/yow-1
```

## Schema

### Required

- `virtual_machine` (String) VM slug.
- `interval` (String) One of `hourly`, `dailyAt`, `weeklyOn`, or `monthlyOn`.
- `at` (String) Whole-hour time in `HH:00` format.
- `timezone` (String) IANA timezone.
- `retention` (Number) Number of backups to retain. Must be at least one.
- `region` (String) Region slug.

### Optional

- `name` (String) Schedule name.
- `description` (String) Schedule description. Changing `name` or `description`
  forces replacement.
- `day` (Number) Weekday 0 through 6 for `weeklyOn`, or day 1 through 28 for
  `monthlyOn`.
- `immediate` (Boolean) Request an immediate backup during creation. Changing
  this field forces replacement.
- `paused` (Boolean) Pause or resume the schedule.
- `project` (String) Project slug. Inherits from `default_project` when omitted.

### Read-Only

- `id` (String) Scheduler policy ID.
- `status` (String) Scheduler status.
