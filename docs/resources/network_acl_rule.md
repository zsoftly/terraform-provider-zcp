---
page_title: "zcp_network_acl_rule Resource"
description: |-
  Create and manage rules in a ZCP Network ACL.
---

# zcp_network_acl_rule

Manages a single rule in a ZCP Network ACL. Rules are independent resources (like `aws_network_acl_rule` and `azurerm_network_security_rule`), so you can manage them individually or with `for_each`. `number`, `action`, `traffic_type`, `protocol`, `cidr_list`, the port range, and the ICMP fields are updated in place; changing `acl` or `vpc` forces replacement.

## Example Usage

```terraform
resource "zcp_network_acl_rule" "https_in" {
  vpc          = zcp_vpc.main.id
  acl          = zcp_network_acl.web.id
  number       = 100
  action       = "allow"
  traffic_type = "ingress"
  protocol     = "tcp"
  cidr_list    = "0.0.0.0/0"
  start_port   = 443
  end_port     = 443
}
```

## Import

Import using `<vpc-slug>/<acl-id>/<rule-id>`:

```shell
terraform import zcp_network_acl_rule.https_in main-vpc/<acl-id>/<rule-id>
```

## Schema

### Required

- `vpc` (String) Slug of the `zcp_vpc` the ACL belongs to. Changing this forces replacement.
- `acl` (String) ID of the `zcp_network_acl` this rule belongs to. Changing this forces replacement.
- `number` (Number) Rule number (order/priority). Must be unique within the ACL.
- `action` (String) Rule action: `allow` or `deny`.
- `traffic_type` (String) Traffic direction: `ingress` or `egress`.
- `protocol` (String) Protocol: `tcp`, `udp`, `icmp`, or `all`.
- `cidr_list` (String) CIDR the rule applies to (e.g. `0.0.0.0/0`).

### Optional

- `start_port` (Number) Start of the port range (required for `tcp`/`udp`).
- `end_port` (Number) End of the port range (required for `tcp`/`udp`).
- `icmp_type` (Number) ICMP type (required for `icmp`).
- `icmp_code` (Number) ICMP code (required for `icmp`).
- `description` (String) Human-readable description of the rule.

### Read-Only

- `id` (String) Rule ID (UUID).
