---
page_title: "zcp_remote_access_vpn Resource"
description: |-
  Enable remote access VPN on a ZCP public IP.
---

# zcp_remote_access_vpn

Enables remote access VPN on a public IP address. Pair with `zcp_vpn_user` for the accounts allowed to connect. Destroy disables the VPN on the IP.

## Example Usage

```terraform
resource "zcp_remote_access_vpn" "office" {
  ip_address = zcp_ip_address.vpn.id
}

resource "zcp_vpn_user" "alice" {
  username       = "alice"
  password       = var.vpn_password
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
}
```

## Import

Import using `<ip-slug>/<vpn-id>`:

```shell
terraform import zcp_remote_access_vpn.office 1036521143/<vpn-id>
```

## Schema

### Required

- `ip_address` (String) Public IP address slug the VPN is enabled on. Changing this forces replacement.

### Read-Only

- `id` (String) Remote access VPN ID.
- `public_ip` (String) Public IP clients connect to.
- `state` (String) Current state of the remote access VPN.
