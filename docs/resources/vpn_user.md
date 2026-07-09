---
page_title: "zcp_vpn_user Resource"
description: |-
  Create and manage ZCP remote access VPN users.
---

# zcp_vpn_user

Manages a remote access VPN user. The API has no update endpoint for VPN users,
so every change forces replacement.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_vpn_user" "alice" {
  username       = "alice"
  password       = var.vpn_password
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
}
```

## Import

Import using the VPN user slug:

```shell
terraform import zcp_vpn_user.alice <vpn-user-slug>
```

## Schema

### Required

- `username` (String) VPN username. Changing this forces replacement.
- `password` (String, Sensitive) VPN user password. Write-only. Changing this
  forces replacement.
- `cloud_provider` (String) Cloud provider slug. Read it from `data.zcp_region`
  instead of hardcoding. Changing this forces replacement.
- `region` (String) Region slug. Changing this forces replacement.

### Optional

- `project` (String) Project slug. Inherits from the provider `default_project`
  if omitted. Changing this forces replacement.
- `timeouts` (Block) Configurable `create` and `delete` timeouts.

### Read-Only

- `id` (String) VPN user slug.
- `status` (String) VPN user status.
