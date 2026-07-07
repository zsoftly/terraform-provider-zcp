---
page_title: "zcp_egress_rule Resource"
description: |-
  Create and manage ZCP egress firewall rules on a network.
---

# zcp_egress_rule

Manages an egress firewall rule on a ZCP network. Egress rules control outbound
traffic from instances in the network to destination CIDRs. The API has no
update endpoint for egress rules, so every change forces replacement.

## Example Usage

```terraform
resource "zcp_egress_rule" "https_out" {
  network    = zcp_network.prod.id
  protocol   = "tcp"
  cidr       = "0.0.0.0/0"
  start_port = "443"
  end_port   = "443"
}
```

## Import

Import using `<network-slug>/<rule-id>`:

```shell
terraform import zcp_egress_rule.https_out prod-net/<rule-id>
```

## Schema

### Required

- `network` (String) Parent network slug. Changing this forces replacement.
- `protocol` (String) Protocol: `tcp`, `udp`, `icmp`, or `all`. Changing this
  forces replacement.

### Optional

- `cidr` (String) Destination CIDR the rule allows traffic to (e.g.
  `0.0.0.0/0`). Changing this forces replacement.
- `start_port` (String) Start of the port range. Changing this forces
  replacement.
- `end_port` (String) End of the port range. Changing this forces replacement.
- `icmp_type` (String) ICMP type, used with `protocol = "icmp"`. Changing this
  forces replacement.
- `icmp_code` (String) ICMP code, used with `protocol = "icmp"`. Changing this
  forces replacement.
- `timeouts` (Block) Configurable `create` and `delete` timeouts.

### Read-Only

- `id` (String) Egress rule ID.
- `state` (String) Current state of the rule.
