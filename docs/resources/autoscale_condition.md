---
page_title: "zcp_autoscale_condition Resource"
description: |-
  Create and manage scale-down conditions on ZCP autoscale groups.
---

# zcp_autoscale_condition

Manages a scale-down condition on a `zcp_autoscale_group`. All rule fields update in place; changing `autoscale_group` forces replacement. For scale-up rules use `zcp_autoscale_policy`.

## Example Usage

```terraform
resource "zcp_autoscale_condition" "cpu_low" {
  autoscale_group = zcp_autoscale_group.web.id
  name            = "cpu-low"
  metric          = "cpu"
  operator        = "LT"
  threshold       = 20
  duration        = 600
  scale_amount    = 1
}
```

## Import

Import using `<group-slug>/<condition-id>`:

```shell
terraform import zcp_autoscale_condition.cpu_low web-asg-a1/21
```

## Schema

### Required

- `autoscale_group` (String) Parent autoscale group slug. Changing this forces replacement.
- `name` (String) Rule name. Updated in place.
- `metric` (String) Metric the rule evaluates (e.g. `cpu`, `memory`). Updated in place.
- `operator` (String) Comparison operator (e.g. `GT`, `LT`). Updated in place.
- `threshold` (Number) Metric threshold triggering the rule. Updated in place.
- `duration` (Number) Seconds the metric must breach the threshold before scaling. Updated in place.
- `scale_amount` (Number) Instances to remove per scaling action. Updated in place.

### Optional

- `cooldown` (Number) Cooldown in seconds after this rule fires. Updated in place.

### Read-Only

- `id` (String) Condition ID.
