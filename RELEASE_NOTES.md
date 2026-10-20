# terraform-provider-zcp v1.1.0 Release Notes

Provider v1.1.0 adds `zcp_volume_snapshot_schedule` for block storage volumes.
It manages scheduled snapshots as Terraform state, including timing, timezone,
retention, pause state, and scope.

Replace `<existing-volume-slug>` with the slug of a volume that already exists
in the selected region and project.

```hcl
data "zcp_region" "yul" {
  slug = "yul-1"
}

resource "zcp_volume_snapshot_schedule" "data" {
  volume    = "<existing-volume-slug>"
  interval  = "dailyAt"
  at        = "02:00"
  timezone  = "America/Toronto"
  retention = 7
  region    = data.zcp_region.yul.slug
}
```

Use `hourly`, `dailyAt`, `weeklyOn`, or `monthlyOn` for `interval`. Set `day` to
`0` through `6` for a weekly schedule or `1` through `28` for a monthly
schedule. `at` accepts whole hours in `HH:00` format.

Import with `<id>/<region>[/<project>]`:

```bash
terraform import zcp_volume_snapshot_schedule.data policy-1/yul-1
```

The first apply after import records the configured `immediate` value without
replacing the schedule or creating a snapshot. Later changes to `immediate`,
`name`, or `description` replace the schedule. The platform rejects deletion
while snapshots remain, so remove those snapshots first. Pause a schedule when
you need to keep its snapshots but stop future runs.

For an incomplete create response with a policy ID, the provider reads the
policy back and verifies its scope before saving it to state. It refuses a
policy with conflicting scope without reading it back and provides recovery
steps instead of managing a possibly different schedule.

## Upgrade

Set the provider constraint to `~> 1.1.0`, then run `terraform init -upgrade` or
`tofu init -upgrade`. Review the plan before applying it.
