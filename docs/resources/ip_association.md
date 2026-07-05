---
page_title: "zcp_ip_association Resource"
description: |-
  Associate an owned public IP with an instance via static NAT.
---

# zcp_ip_association

Associates an owned public IP (`zcp_ip_address`) with an instance via static NAT. This is the ZCP analog of `aws_eip_association` (and the Azure public-IP association). You acquire and own an IP as its own resource, then attach it to an instance with a separate association resource.

The `network` argument is the network the instance is on (a VPC tier or a standalone `zcp_network`), so the same resource covers both topologies. Association is create/delete only. Static NAT is enabled on create and removed on destroy. Changing any argument forces replacement.

## Example Usage

```terraform
resource "zcp_ip_address" "eip" {
  vpc           = zcp_vpc.main.id
  plan          = "ipv4-yow"
  billing_cycle = "hourly"
}

resource "zcp_ip_association" "web" {
  ip_address      = zcp_ip_address.eip.id
  virtual_machine = zcp_instance.web.id
  network         = zcp_network.tier.id
}
```

## Import

Import using `<ip-slug>/<vm-slug>/<network-slug>`:

```shell
terraform import zcp_ip_association.web <ip-slug>/<vm-slug>/<network-slug>
```

## Schema

### Required

- `ip_address` (String) Slug of the owned `zcp_ip_address` to associate. Changing this forces replacement.
- `virtual_machine` (String) Slug of the `zcp_instance` to associate the IP with. Changing this forces replacement.
- `network` (String) Slug of the network the instance is on (a VPC tier or a standalone `zcp_network`). Changing this forces replacement.

### Read-Only

- `id` (String) Association ID (the IP slug; an IP has at most one static-NAT association).
