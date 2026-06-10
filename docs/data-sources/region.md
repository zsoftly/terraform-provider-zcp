---
page_title: "zcp_region Data Source"
description: |-
  Look up a ZCP region by slug.
---

# zcp_region

Look up a ZCP region by slug and expose its attributes as read-only values. Use the `slug` output in resource `region` arguments to reference a region without hard-coding its ID.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

output "region_name" {
  value = data.zcp_region.yow.name
}
```

## Schema

### Required

- `slug` (String) Unique region slug (e.g. `yow-1`).

### Read-Only

- `id` (String) Region ID.
- `name` (String) Region display name.
- `country` (String) Full country name.
- `country_code` (String) ISO country code.
