---
page_title: "zcp_network Data Source"
description: |-
  Look up an existing ZCP network by slug.
---

# zcp_network (Data Source)

Looks up an existing network by slug, for example to place instances into a network created outside Terraform.

## Example Usage

```terraform
data "zcp_network" "prod" {
  slug   = "prod-net"
  region = "yow-1"
}

resource "zcp_instance" "web" {
  # ...
  network = data.zcp_network.prod.id
}
```

## Schema

### Required

- `slug` (String) Network slug.

### Optional

- `region` (String) Region slug to scope the lookup (e.g. `yow-1`).
- `project` (String) Project slug to scope the lookup. Inherits from the provider `default_project` if omitted.

### Read-Only

- `id` (String) Network slug (same as `slug`).
- `name` (String) Network display name.
- `type` (String) Network type (e.g. `Isolated`, `L2`).
- `gateway` (String) Gateway address.
- `cidr` (String) Network CIDR.
- `netmask` (String) Netmask.
- `category` (String) Network category.
- `vpc` (String) Parent VPC slug when the network is a VPC tier.
- `is_default` (Boolean) Whether this is the default network.
- `zone_name` (String) Zone name.
- `description` (String) Network description.
