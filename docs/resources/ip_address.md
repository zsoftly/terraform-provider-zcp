---
page_title: "zcp_ip_address Resource"
description: |-
  Allocate and manage ZCP public IP addresses.
---

# zcp_ip_address

Allocates a public IP address into a VPC or network. The API has no update
endpoint for IP addresses, so every change forces replacement. Bind the IP to an
instance with `zcp_ip_association`, then open ports with `zcp_firewall_rule` or
`zcp_port_forward`.

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

resource "zcp_ip_association" "web" {
  ip_address      = zcp_ip_address.web.id
  virtual_machine = zcp_instance.web.id
  network         = zcp_network.prod.id
}
```

## Import

Import using the IP address slug:

```shell
terraform import zcp_ip_address.web 1036521143
```

## Schema

### Required

- `plan` (String) Plan slug for the IP address (e.g. `public-ip-1`). Changing
  this forces replacement.
- `billing_cycle` (String) Billing cycle (e.g. `hourly`, `monthly`). Changing
  this forces replacement.

### Optional

- `vpc` (String) VPC slug to associate with the IP. At least one of `vpc` or
  `network` must be set. Changing this forces replacement.
- `network` (String) Network slug to associate with the IP. At least one of
  `vpc` or `network` must be set. Changing this forces replacement.
- `project` (String) Project slug. Inherits from the provider `default_project`
  if omitted. Changing this forces replacement.
- `timeouts` (Block) Configurable `create` and `delete` timeouts.

### Read-Only

- `id` (String) IP address slug.
- `ip_address` (String) The allocated public IP address.
- `type` (String) IP address type (e.g. `Public`).
