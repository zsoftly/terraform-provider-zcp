---
page_title: "zcp_volume_snapshot_schedule Resource"
description: |-
  Create and manage block storage snapshot schedules.
---

# zcp_volume_snapshot_schedule

Manages a block storage snapshot schedule. Schedule updates change its timing,
timezone, and retention. Set `paused` to pause or resume a schedule. Terraform
does not model one-off snapshot runs as durable state.

## Example Usage

```terraform
resource "zcp_volume_snapshot_schedule" "data" {
  volume    = zcp_volume.data.id
  interval  = "dailyAt"
  at        = "02:00"
  timezone  = "America/Toronto"
  retention = 7
  region    = data.zcp_region.yow.slug
}
```

## Import

Import using `<id>/<region>[/<project>]`.

```shell
terraform import zcp_volume_snapshot_schedule.data policy-1/yow-1
```

## Schema

### Required

- `volume` (String) Block storage volume slug.
- `interval` (String) One of `hourly`, `dailyAt`, `weeklyOn`, or `monthlyOn`.
- `at` (String) Whole-hour time in `HH:00` format.
- `timezone` (String) IANA timezone.
- `retention` (Number) Number of snapshots to retain. Must be at least one.
- `region` (String) Region slug.

### Optional

- `name` (String) Schedule name.
- `description` (String) Schedule description. Changing `name` or `description`
  forces replacement.
- `day` (Number) Weekday 0 through 6 for `weeklyOn`, or day 1 through 28 for
  `monthlyOn`.
- `immediate` (Boolean) Request an immediate snapshot during creation. Changing
  this field forces replacement.
- `paused` (Boolean) Pause or resume the schedule.
- `project` (String) Project slug. Inherits from `default_project` when omitted.

### Read-Only

- `id` (String) Scheduler policy ID.
- `status` (String) Scheduler status.
