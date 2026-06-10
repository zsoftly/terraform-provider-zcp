---
page_title: "zcp_template Data Source"
description: |-
  Look up a ZCP public VM template by slug.
---

# zcp_template

Look up a ZCP public VM template by slug. Optionally narrow the search to a specific region to avoid ambiguity when the same template slug exists in multiple regions.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

data "zcp_template" "ubuntu" {
  slug        = "ubuntu-2204-lts"
  region_slug = data.zcp_region.yow.slug
}

output "template_id" {
  value = data.zcp_template.ubuntu.id
}
```

## Schema

### Required

- `slug` (String) Unique template slug (e.g. `ubuntu-2204-lts`).

### Optional

- `region_slug` (String) Narrow the search to a specific region slug. Recommended to avoid ambiguity when the same template exists in multiple regions.

### Read-Only

- `id` (String) Template ID.
- `name` (String) Template display name.
- `region_id` (String) ID of the region this template belongs to.
- `type` (String) Template type (e.g. `Template`).
