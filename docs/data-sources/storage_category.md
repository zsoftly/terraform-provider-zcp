---
page_title: "zcp_storage_category Data Source"
description: |-
  Look up a ZCP storage category by slug.
---

# zcp_storage_category (Data Source)

Looks up a storage category by slug. Categories are region-specific (e.g. `nvme`
in YOW, `pro-nvme` in YUL), so pass `region` to scope the lookup.

## Example Usage

```terraform
data "zcp_storage_category" "nvme" {
  slug   = "pro-nvme"
  region = "yul-1"
}

resource "zcp_instance" "web" {
  # ...
  storage_category = data.zcp_storage_category.nvme.slug
}
```

## Schema

### Required

- `slug` (String) Storage category slug (e.g. `nvme`, `pro-nvme`).

### Optional

- `region` (String) Region slug to scope the lookup (e.g. `yow-1`).

### Read-Only

- `id` (String) Storage category ID.
- `name` (String) Storage category display name.
- `status` (Boolean) Whether the storage category is enabled.
