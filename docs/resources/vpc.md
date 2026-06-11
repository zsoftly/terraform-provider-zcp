---
page_title: "zcp_vpc Resource"
description: |-
  Create and manage ZCP Virtual Private Clouds.
---

# zcp_vpc

Manages a ZCP Virtual Private Cloud (VPC). A VPC provides an isolated network environment with its own routing, ACLs, and optional VPN gateway.

`cloud_provider`, `region`, `cidr`, `size`, `project`, `type`, `billing_cycle`, and `plan` are immutable after creation — changing any of these forces replacement. `name` and `description` can be updated in-place.

~> **Note on write-only fields:** `cloud_provider`, `region`, `project`, `type`, `billing_cycle`, `plan`, and `size` are sent to the API on creation but are not included in the VPC read response. They are preserved in Terraform state but cannot be verified on refresh.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

# cloud_provider is read from the region — no hardcoding required.
resource "zcp_vpc" "main" {
  name           = "main-vpc"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  cidr           = "10.0.0.0/22"
  size           = "small"
  description    = "Primary VPC"
}
```

## Import

Import an existing VPC by its slug:

```shell
terraform import zcp_vpc.main <slug>
```

After import, write-only fields (`cloud_provider`, `region`, `project`, `size`, `type`, `billing_cycle`, `plan`) will be null in state. Populate them in your configuration to avoid perpetual diffs or unintended replacements.

## Schema

### Required

- `name` (String) Display name for the VPC.
- `cloud_provider` (String) Cloud provider slug (e.g. `cloudstack`). Changing this forces replacement.
- `region` (String) Region slug where the VPC is created. Changing this forces replacement.
- `cidr` (String) Network address for the VPC (e.g. `10.1.0.1`). This is the base IP address — do not include the prefix length. Changing this forces replacement.
- `size` (String) Subnet mask prefix length as a string (e.g. `"24"` for /24, `"16"` for /16). Changing this forces replacement.

### Optional

- `project` (String) Project slug. Inherits from the provider `default_project` if omitted. Changing this forces replacement.
- `description` (String) Human-readable description.
- `type` (String) VPC type (e.g. `Vpc`). Changing this forces replacement.
- `billing_cycle` (String) Billing cycle (`hourly` or `monthly`). Changing this forces replacement.
- `plan` (String) Plan slug for VPC compute resources. Run `zcp plan router` to list available plans (e.g. `virtual-private-cloud-vpc-1` for 5 Gbps, `virtual-private-cloud-vpc` for 50 Mbps). Changing this forces replacement.
- `storage_category` (String) Storage category slug. Run `zcp storage-category list` to list available values (e.g. `nvme`, `pro-nvme`, `premium-ssd`). Changing this forces replacement.

### Read-Only

- `id` (String) VPC slug (unique identifier).
- `status` (String) Current VPC status.
