---
page_title: "zcp_billing_cycle Data Source"
description: |-
  Look up a ZCP billing cycle by slug.
---

# zcp_billing_cycle (Data Source)

Looks up a billing cycle by slug, for use in resources requiring a
`billing_cycle`.

## Example Usage

```terraform
data "zcp_billing_cycle" "hourly" {
  slug = "hourly"
}

resource "zcp_volume" "data" {
  # ...
  billing_cycle = data.zcp_billing_cycle.hourly.slug
}
```

## Schema

### Required

- `slug` (String) Billing cycle slug (e.g. `hourly`, `monthly`).

### Read-Only

- `id` (String) Billing cycle ID.
- `name` (String) Billing cycle display name.
- `description` (String) Billing cycle description.
- `duration` (Number) Cycle duration in `unit`s.
- `unit` (String) Duration unit (e.g. `hour`, `month`).
- `is_enabled` (Boolean) Whether the billing cycle is enabled.
