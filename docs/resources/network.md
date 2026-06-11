---
page_title: "zcp_network Resource"
description: |-
  Create and manage ZCP networks.
---

# zcp_network

Manages a ZCP network. Networks provide L2/L3 connectivity for virtual machines within a region.

`cloud_provider`, `region`, `project`, and `category_slug` are immutable after creation — changing any of these forces replacement. `name` and `description` can be updated in-place.

~> **Note on `category_slug`:** The ZCP API does not return this field after creation. It is preserved in Terraform state but cannot be verified on subsequent reads. Changes to this field force replacement.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

# cloud_provider is read from the region — no hardcoding required.
resource "zcp_network" "app" {
  name           = "app-network"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  category_slug  = "isolated"
  description    = "Application tier network"
}
```

## Import

Import an existing network by its slug:

```shell
terraform import zcp_network.app <slug>
```

After import, `cloud_provider`, `region`, `project`, and `category_slug` will be null in state. Populate them in your configuration to avoid perpetual diffs.

## Schema

### Required

- `name` (String) Display name for the network.
- `cloud_provider` (String) Cloud provider slug (e.g. `cloudstack`). Changing this forces replacement.
- `region` (String) Region slug where the network is created. Changing this forces replacement.

### Optional

- `project` (String) Project slug. Inherits from the provider `default_project` if omitted. Changing this forces replacement.
- `description` (String) Human-readable description.
- `category_slug` (String) Network category slug. Not returned by the API after creation; changes force replacement.

### Read-Only

- `id` (String) Network slug (unique identifier).
- `gateway` (String) Network gateway IP address.
- `cidr` (String) Network CIDR block.
- `netmask` (String) Network subnet mask.
