---
page_title: "zcp_volume Data Source"
description: |-
  Look up an existing ZCP block storage volume by slug.
---

# zcp_volume (Data Source)

Looks up an existing block storage volume by slug, for example to find the slug
of an instance's root volume for `zcp_volume_backup` without hardcoding it.

The volume lookup retrieves every page of block storage results. `region` and
`project` narrow the lookup. Results are complete within that scope.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

data "zcp_volume" "data_disk" {
  slug   = "data-1234"
  region = data.zcp_region.yow.slug
}
```

## Schema

### Required

- `slug` (String) Volume slug.

### Optional

- `region` (String) Region slug to scope the lookup (e.g. `yow-1`).
- `project` (String) Project slug to scope the lookup. Inherits from the
  provider `default_project` if omitted.

### Read-Only

- `id` (String) Volume slug (same as `slug`).
- `name` (String) Volume display name.
- `size` (Number) Storage size in GB.
- `volume_type` (String) Volume type (`ROOT` for a boot disk, empty for a data
  volume).
- `instance_id` (String) ID of the virtual machine this volume is attached to,
  empty when detached.
- `created_at` (String) Timestamp the volume was created.
