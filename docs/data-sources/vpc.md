---
page_title: "zcp_vpc Data Source"
description: |-
  Look up an existing ZCP VPC by slug.
---

# zcp_vpc (Data Source)

Looks up an existing VPC by slug, for example to add tiers or ACLs to a VPC created outside Terraform.

## Example Usage

```terraform
data "zcp_vpc" "main" {
  slug   = "main-vpc"
  region = "yow-1"
}

resource "zcp_network_acl" "web" {
  vpc = data.zcp_vpc.main.id
  # ...
}
```

## Schema

### Required

- `slug` (String) VPC slug.

### Optional

- `region` (String) Region slug to scope the lookup (e.g. `yow-1`).
- `project` (String) Project slug to scope the lookup. Inherits from the provider `default_project` if omitted.

### Read-Only

- `id` (String) VPC slug (same as `slug`).
- `name` (String) VPC display name.
- `description` (String) VPC description.
- `status` (String) Current VPC status.
- `cidr` (String) VPC CIDR.
- `zone_name` (String) Zone name.
- `domain_name` (String) Network domain name.
