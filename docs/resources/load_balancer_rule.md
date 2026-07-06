---
page_title: "zcp_load_balancer_rule Resource"
description: |-
  Create and manage additional rules on a ZCP load balancer.
---

# zcp_load_balancer_rule

Manages an additional rule on a `zcp_load_balancer`. The initial rule is part of the load balancer resource itself; use this resource for every rule beyond the first. The API has no update endpoint for rules, so every change forces replacement.

## Example Usage

```terraform
resource "zcp_load_balancer_rule" "http" {
  load_balancer = zcp_load_balancer.web.id
  name          = "http"
  public_port   = "80"
  private_port  = "8080"
  algorithm     = "roundrobin"
}
```

## Import

Import using `<load-balancer-slug>/<rule-id>`:

```shell
terraform import zcp_load_balancer_rule.http web-lb-a1b2/<rule-id>
```

## Schema

### Required

- `load_balancer` (String) Parent load balancer slug. Changing this forces replacement.
- `name` (String) Rule name, unique within the load balancer. Changing this forces replacement.
- `public_port` (String) Public port (e.g. `80`). Changing this forces replacement.
- `private_port` (String) Private port (e.g. `8080`). Changing this forces replacement.
- `algorithm` (String) Balancing algorithm: `roundrobin`, `leastconn`, or `source`. Changing this forces replacement.

### Optional

- `protocol` (String) Protocol (e.g. `tcp`). Changing this forces replacement.
- `sticky_method` (String) Session stickiness method (e.g. `LbCookie`, `SourceBased`). Changing this forces replacement.
- `enable_tls` (Boolean) Enable TLS on the rule. Changing this forces replacement.
- `enable_proxy` (Boolean) Enable the PROXY protocol on the rule. Changing this forces replacement.

### Read-Only

- `id` (String) Rule ID.
