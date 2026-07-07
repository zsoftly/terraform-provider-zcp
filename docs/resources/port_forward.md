---
page_title: "zcp_port_forward Resource"
description: |-
  Create and manage ZCP port forwarding rules.
---

# zcp_port_forward

Manages a port forwarding rule on a ZCP public IP address, forwarding a public
port range to an instance. The API has no update endpoint for these rules, so
every change forces replacement.

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

resource "zcp_instance" "web" {
  name             = "web-01"
  template         = "ubuntu-2604-lts-1"
  plan             = "ca2sl"
  billing_cycle    = "hourly"
  network          = zcp_network.prod.id
  storage_category = "nvme"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
}

resource "zcp_ip_address" "web" {
  plan          = "public-ip-1"
  billing_cycle = "hourly"
  network       = zcp_network.prod.id
}

resource "zcp_port_forward" "ssh" {
  ip_address         = zcp_ip_address.web.id
  protocol           = "tcp"
  public_start_port  = "22"
  private_start_port = "22"
  virtual_machine    = zcp_instance.web.id
}
```

## Import

Import using `<ip-address-slug>/<rule-id>`:

```shell
terraform import zcp_port_forward.ssh 1036521143/<rule-id>
```

## Schema

### Required

- `ip_address` (String) Parent IP address slug. Changing this forces
  replacement.
- `protocol` (String) IP protocol (`tcp` or `udp`). Changing this forces
  replacement.
- `public_start_port` (String) First public port in the range. Changing this
  forces replacement.
- `private_start_port` (String) First private (VM-side) port in the range.
  Changing this forces replacement.
- `virtual_machine` (String) Instance slug to forward traffic to. Write-only.
  Changing this forces replacement.

### Optional

- `public_end_port` (String) Last public port in the range. Omit for a
  single-port rule. Changing this forces replacement.
- `private_end_port` (String) Last private port in the range. Omit for a
  single-port rule. Changing this forces replacement.
- `timeouts` (Block) Configurable `create` and `delete` timeouts.

### Read-Only

- `id` (String) Port forwarding rule UUID.
- `state` (String) Current state of the rule.
