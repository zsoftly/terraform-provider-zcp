---
page_title: "zcp_firewall_rule Resource"
description: |-
  Create and manage ZCP firewall rules on a public IP address.
---

# zcp_firewall_rule

Manages an ingress firewall rule on a ZCP public IP address. The API has no
update endpoint for firewall rules, so every change forces replacement. For
outbound rules on a network use `zcp_egress_rule`.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_network" "prod" {
  name           = "prod-network"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  network_plan   = "inet-yow"
  billing_cycle  = "hourly"
}

resource "zcp_ip_address" "web" {
  plan          = "public-ip-1"
  billing_cycle = "hourly"
  network       = zcp_network.prod.id
}

resource "zcp_firewall_rule" "https_in" {
  ip_address = zcp_ip_address.web.id
  protocol   = "tcp"
  cidr_list  = "0.0.0.0/0"
  start_port = "443"
  end_port   = "443"
}
```

## Import

Import using `<ip-address-slug>/<rule-id>`:

```shell
terraform import zcp_firewall_rule.https_in 1036521143/<rule-id>
```

## Schema

### Required

- `ip_address` (String) Parent IP address slug. Changing this forces
  replacement.
- `protocol` (String) Protocol: `tcp`, `udp`, `icmp`, or `all`. Changing this
  forces replacement.

### Optional

- `cidr_list` (String) Comma-separated list of source CIDRs (e.g. `0.0.0.0/0`).
  Changing this forces replacement.
- `destination_cidr_list` (String) Comma-separated list of destination CIDRs.
  Changing this forces replacement.
- `start_port` (String) Start of the port range. Changing this forces
  replacement.
- `end_port` (String) End of the port range. Changing this forces replacement.
- `timeouts` (Block) Configurable `create` and `delete` timeouts.

### Read-Only

- `id` (String) Firewall rule ID.
- `state` (String) Current state of the rule.
