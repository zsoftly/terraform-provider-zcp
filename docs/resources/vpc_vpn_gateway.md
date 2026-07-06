---
page_title: "zcp_vpc_vpn_gateway Resource"
description: |-
  Create and manage ZCP VPN gateways on a VPC.
---

# zcp_vpc_vpn_gateway

Manages a site-to-site VPN gateway attached to a VPC. The gateway is the platform-side endpoint of an IPSec tunnel; pair it with a `zcp_vpn_customer_gateway` for the remote side. All attributes force replacement.

## Example Usage

```terraform
resource "zcp_vpc_vpn_gateway" "main" {
  vpc = zcp_vpc.main.id
}
```

## Import

Import using `<vpc-slug>/<gateway-slug>`:

```shell
terraform import zcp_vpc_vpn_gateway.main main-vpc/<gateway-slug>
```

## Schema

### Required

- `vpc` (String) Parent VPC slug. Changing this forces replacement.

### Read-Only

- `id` (String) VPN gateway slug.
- `public_ip` (String) Public IP address assigned to the VPN gateway.
- `status` (String) Current status of the VPN gateway.
- `zone_name` (String) Zone in which the VPN gateway is deployed.
