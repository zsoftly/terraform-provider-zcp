---
page_title: "zcp_volume_snapshot Resource"
description: |-
  Create and manage ZCP block storage snapshots.
---

# zcp_volume_snapshot

Manages a snapshot of a ZCP block storage volume. Snapshots are immutable, so every change forces replacement. Reverting a volume is an operational action outside Terraform (`zcp snapshot revert`).

## Example Usage

```terraform
resource "zcp_volume_snapshot" "data_nightly" {
  volume         = zcp_volume.data.id
  name           = "data-nightly"
  billing_cycle  = "hourly"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
}
```

## Import

Import using `<slug>/<region>/<cloud_provider>/<volume>/<billing_cycle>[/<plan>/<project>]`:

```shell
terraform import zcp_volume_snapshot.data_nightly data-nightly-s1/yow-1/zsoftly/data-1234/hourly
```

## Schema

### Required

- `volume` (String) Slug of the block storage volume to snapshot. Changing this forces replacement.
- `name` (String) Snapshot name. Changing this forces replacement.
- `billing_cycle` (String) Billing cycle (e.g. `hourly`, `monthly`). Changing this forces replacement.
- `cloud_provider` (String) Cloud provider slug. Changing this forces replacement.
- `region` (String) Region slug (e.g. `yow-1`). Changing this forces replacement.

### Optional

- `plan` (String) Snapshot plan slug. Changing this forces replacement.
- `project` (String) Project slug. Inherits from the provider `default_project` if omitted. Changing this forces replacement.

### Read-Only

- `id` (String) Snapshot slug.
