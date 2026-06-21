---
page_title: "zcp_plan Data Source"
description: |-
  Look up a ZCP service plan by slug.
---

# zcp_plan

Look up a ZCP service plan by slug. Use the `id` output to reference the plan in resource configurations. The `service` argument scopes the search to a specific service type (defaults to `Virtual Machine`).

## Example Usage

```terraform
data "zcp_plan" "small" {
  slug = "ci1xs"
}

output "plan_cpu" {
  value = data.zcp_plan.small.cpu
}

output "plan_monthly" {
  value = data.zcp_plan.small.monthly_price
}
```

## Schema

### Required

- `slug` (String) Unique plan slug (e.g. `ci1xs`).

### Optional

- `service` (String) Service type to search. Defaults to `Virtual Machine`. Accepted values: `Virtual Machine`, `Virtual Router`, `Block Storage`, `Load Balancer`, `Kubernetes`, `IP Address`, `VM Snapshot`, `My Template`, `ISO`, `Backups`.

### Read-Only

- `id` (String) Plan ID.
- `name` (String) Plan display name.
- `cpu` (Number) Number of vCPUs.
- `memory_mb` (Number) RAM in megabytes.
- `storage_gb` (Number) Root disk size in gigabytes.
- `hourly_price` (Number) Hourly price in account currency.
- `monthly_price` (Number) Monthly price in account currency.
