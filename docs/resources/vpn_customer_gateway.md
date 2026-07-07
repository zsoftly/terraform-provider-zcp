---
page_title: "zcp_vpn_customer_gateway Resource"
description: |-
  Create and manage ZCP site-to-site VPN customer gateways.
---

# zcp_vpn_customer_gateway

Manages a site-to-site VPN customer gateway describing the remote end of an
IPSec tunnel. `name`, the IKE/ESP policies, and the tunnel flags update in
place.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_vpn_customer_gateway" "office" {
  name           = "office"
  gateway        = "203.0.113.99"
  cidr_list      = "192.168.10.0/24"
  ipsec_psk      = var.office_psk
  ike_policy     = "aes128-sha1-dh5"
  esp_policy     = "aes128-sha1"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
}
```

## Import

Import using the customer gateway slug:

```shell
terraform import zcp_vpn_customer_gateway.office <gateway-slug>
```

The write-only attributes (`ipsec_psk`, algorithm fields) are not returned by
the API; set them in config before importing to avoid a follow-up diff.

## Schema

### Required

- `name` (String) Display name. Updated in place.
- `gateway` (String) Remote gateway IP address.
- `cidr_list` (String) Comma-separated list of CIDRs reachable behind the remote
  gateway.
- `ipsec_psk` (String, Sensitive) IPSec pre-shared key. Write-only.
- `ike_policy` (String) IKE policy string (e.g. `aes128-sha1-dh5`). Updated in
  place.
- `esp_policy` (String) ESP policy string (e.g. `aes128-sha1`). Updated in
  place.
- `cloud_provider` (String) Cloud provider slug. Read it from `data.zcp_region`
  instead of hardcoding.
- `region` (String) Region slug (e.g. `yow-1`).

### Optional

- `ike_lifetime` (String) IKE SA lifetime in seconds (e.g. `86400`).
- `esp_lifetime` (String) ESP SA lifetime in seconds (e.g. `3600`).
- `ike_encryption` (String) IKE encryption algorithm (e.g. `aes128`).
  Write-only.
- `ike_hash` (String) IKE hash algorithm (e.g. `sha1`). Write-only.
- `ike_version` (String) IKE version: `ike`, `ikev1`, or `ikev2`.
- `ike_dh` (String) IKE Diffie-Hellman group (e.g. `modp2048`). Write-only.
- `esp_encryption` (String) ESP encryption algorithm. Write-only.
- `esp_hash` (String) ESP hash algorithm. Write-only.
- `esp_dh` (String) ESP Diffie-Hellman group. Write-only.
- `esp_pfs` (String) ESP Perfect Forward Secrecy group. Write-only.
- `force_encapsulation` (Boolean) Force UDP encapsulation for NAT traversal.
  Updated in place.
- `split_connections` (Boolean) Enable split-connection mode. Updated in place.
- `dead_peer_detection` (Boolean) Enable Dead Peer Detection. Updated in place.
- `project` (String) Project slug. Inherits from the provider `default_project`
  if omitted. Changing this forces replacement.
- `timeouts` (Block) Configurable `create`, `update`, and `delete` timeouts.

### Read-Only

- `id` (String) Customer gateway slug.
