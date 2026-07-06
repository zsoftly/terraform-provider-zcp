---
page_title: "zcp_budget_alert Resource"
description: |-
  Manage the account's budget alert settings.
---

# zcp_budget_alert

Manages the account's budget alert. The account has a single budget alert setting, so declare at most one of this resource. Destroy disables the alert without clearing the configured amounts.

## Example Usage

```terraform
resource "zcp_budget_alert" "main" {
  amount    = 500
  threshold = 80
}
```

## Import

Import with any ID; the account has one budget alert:

```shell
terraform import zcp_budget_alert.main budget-alert
```

## Schema

### Required

- `amount` (Number) Monthly budget amount in account currency. Updated in place.
- `threshold` (Number) Alert threshold as a percentage of the budget (e.g. `80`). Updated in place.

### Optional

- `enabled` (Boolean) Whether the alert is active. Defaults to `true`. Updated in place.

### Read-Only

- `id` (String) Fixed identifier (`budget-alert`).
