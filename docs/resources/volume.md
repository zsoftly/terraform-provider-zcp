---
page_title: "zcp_volume Resource"
description: |-
  Create and manage ZCP block storage volumes.
---

# zcp_volume

Manages a ZCP block storage volume. Provide **exactly one** of `plan` (a fixed offering) or `size` (a custom size in GB). Setting both, or neither, is a plan-time error.

Set `vm` to attach the volume to an instance on creation. Changing `vm` later attaches or detaches the volume in place. Clearing it detaches. On destroy, the provider detaches an attached volume automatically before deletion.

`name`, `cloud_provider`, `region`, `billing_cycle`, `storage_category`, `plan`, `size`, and `project` are immutable. Changing any of them forces replacement. Only `vm` is updatable in place.

~> **Note on write-only fields:** the create-only inputs above are sent to the API on creation but are not all returned in a comparable form by the list response. They are preserved in Terraform state.

~> **Plan vs. size:** at the time of writing, plan-based creation returns a server-side error on the public platform (`Undefined property: stdClass::$storage`), so prefer `size` until that is resolved. The `size` attribute is `Optional`+`Computed`: set it for a custom-size volume, or omit it (with `plan`) to have it computed from the plan.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

# Size-based volume (recommended while plan-based creation is broken server-side).
resource "zcp_volume" "data" {
  name             = "data-vol"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  billing_cycle    = "hourly"
  storage_category = "nvme"
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

`<slug>/<cloud_provider>/<region>/<billing_cycle>/<storage_category>` are required. `size` cannot be imported via the ID (it is numeric). Set it in configuration for size-based volumes. `name` and `slug` come from the subsequent read.

## Schema

### Required

- `name` (String) Display name for the volume. Changing this forces replacement.
- `cloud_provider` (String) Cloud provider slug. Use `data.zcp_region.<name>.cloud_provider`. Changing this forces replacement.
- `region` (String) Region slug (e.g. `yow-1`). Changing this forces replacement.
- `billing_cycle` (String) Billing cycle (`hourly` or `monthly`). Changing this forces replacement.
- `storage_category` (String) Storage category slug. Region-specific: `nvme`/`hdd-storage` in yow-1, `pro-nvme`/`premium-ssd` in yul-1. Changing this forces replacement.

### Optional

- `plan` (String) Plan slug (e.g. `b1g1`). Mutually exclusive with `size`. Changing this forces replacement.
- `size` (Number) Custom storage size in GB. Mutually exclusive with `plan`. Changing this forces replacement.
- `project` (String) Project slug. Inherits from the provider `default_project` if omitted. Changing this forces replacement.
- `vm` (String) Virtual machine slug to attach the volume to. Set, change, or clear this to attach/detach the volume in place.
- `timeouts` (Block) Configurable `create`, `update`, and `delete` timeouts.

### Read-Only

- `id` (String) Volume slug (unique identifier).
- `slug` (String) Volume slug (same value as `id`).
