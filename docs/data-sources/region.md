---
page_title: "zcp_region Data Source"
description: |-
  Look up a ZCP region by slug.
---

# zcp_region

Look up a ZCP region by slug and expose its attributes as read-only values. Use the `slug` output in resource `region` arguments to reference a region without hard-coding its ID.

## Example Usage

```terraform
# Compute regions: "yow-1" (Ottawa) and "yul-1" (Montreal).
# cloud_provider resolves to "nimbo" for both — always read it from the
# data source rather than hardcoding.
data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_network" "app" {
  name           = "app-network"
  cloud_provider = data.zcp_region.yow.cloud_provider  # "nimbo"
  region         = data.zcp_region.yow.slug             # "yow-1"
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
- `cloud_provider` (String) Cloud provider slug for this region (e.g. `nimbo` for compute regions). Pass this to the `cloud_provider` argument of `zcp_network` and `zcp_vpc` resources.
