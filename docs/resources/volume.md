---
page_title: "zcp_volume Resource"
description: |-
  Create and manage ZCP block storage volumes.
---

# zcp_volume

Manages a ZCP block storage volume. Provide `plan`, `size`, or both. A plan
selects the storage tier. A size selects capacity in GB. At least one is
required.

Set `vm` to attach the volume to an instance on creation. Changing `vm` later
attaches or detaches the volume in place. Clearing it detaches. On destroy, the
provider detaches an attached volume automatically before deletion.

`name`, `cloud_provider`, `region`, `billing_cycle`, `storage_category`, `plan`,
`size`, and `project` are immutable. Changing any of them forces replacement.
Only `vm` is updatable in place.

~> **Note on write-only fields:** the create-only inputs above are sent to the
API on creation but are not all returned in a comparable form by the list
response. They are preserved in Terraform state.

~> **Plan and size:** set both to select capacity in a named storage tier. Set
only `plan` when the tier provides capacity, or set only `size` for a
custom-size volume. Use an explicit size for capacity-based tiers. The `size`
attribute is `Optional`+`Computed`.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

# Named storage tier with explicit capacity.
resource "zcp_volume" "data" {
  name             = "data-vol"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  billing_cycle    = "hourly"
  storage_category = "nvme"
  plan             = "b1g1"
  size             = 20
}

# Custom-size volume attached to an instance.
resource "zcp_volume" "db_data" {
  name             = "db-data"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  billing_cycle    = "hourly"
  storage_category = "nvme"
  size             = 100
  vm               = zcp_instance.db.id
}
```

## Import

```shell
terraform import zcp_volume.data '<slug>/<cloud_provider>/<region>/<billing_cycle>/<storage_category>[/<plan>/<project>/<vm>]'
```

`<slug>/<cloud_provider>/<region>/<billing_cycle>/<storage_category>` are
required. `size` cannot be imported via the ID because it is numeric. For a tier
and capacity, include `plan` in the import ID and set `size` in configuration.
`name` and `slug` come from the subsequent read.

## Schema

### Required

- `name` (String) Display name for the volume. Changing this forces replacement.
- `cloud_provider` (String) Cloud provider slug. Use
  `data.zcp_region.<name>.cloud_provider`. Changing this forces replacement.
- `region` (String) Region slug (e.g. `yow-1`). Changing this forces
  replacement.
- `billing_cycle` (String) Billing cycle (`hourly` or `monthly`). Changing this
  forces replacement.
- `storage_category` (String) Storage category slug. Region-specific:
  `nvme`/`hdd-storage` in yow-1, `pro-nvme`/`premium-ssd` in yul-1. Changing
  this forces replacement.

### Optional

- `plan` (String) Plan slug (e.g. `b1g1`). Set `size` with a plan to select
  capacity. Changing this forces replacement.
- `size` (Number) Storage capacity in GB. Set this with a plan to select
  capacity, or without a plan for a custom-size volume. Changing this forces
  replacement.
- `project` (String) Project slug. Inherits from the provider `default_project`
  if omitted. Changing this forces replacement.
- `vm` (String) Virtual machine slug to attach the volume to. Set, change, or
  clear this to attach/detach the volume in place.
- `timeouts` (Block) Configurable `create`, `update`, and `delete` timeouts.

### Read-Only

- `id` (String) Volume slug (unique identifier).
- `slug` (String) Volume slug (same value as `id`).
