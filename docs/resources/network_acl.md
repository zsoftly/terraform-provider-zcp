---
page_title: "zcp_network_acl Resource"
description: |-
  Create and manage ZCP Network ACLs inside a VPC.
---

# zcp_network_acl

Manages a ZCP Network ACL, a stateless allow/deny rule set that lives inside a VPC. Add rules with `zcp_network_acl_rule` and attach the ACL to a subnet with the `acl` argument on `zcp_network`. This mirrors `aws_network_acl` / `azurerm_network_security_group`. The ACL is a container, rules are separate resources, and you wire them together by reference.

~> **No update endpoint:** the API cannot update an ACL in place, so changing `name`, `vpc`, or `description` forces replacement (which recreates its rules and reattaches them via the dependency graph).

## Example Usage

```terraform
resource "zcp_vpc" "main" {
  name           = "main-vpc"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  cidr           = "10.1.0.1"
  size           = "22"
  billing_cycle  = "hourly"
  plan           = "virtual-private-cloud-vpc"
}

resource "zcp_network_acl" "web" {
  name = "web-acl"
  vpc  = zcp_vpc.main.id
}
```

## Import

Import using `<vpc-slug>/<acl-id>` (the VPC slug is needed for every ACL API call):

```shell
terraform import zcp_network_acl.web main-vpc/<acl-id>
```

## Schema

### Required

- `name` (String) Display name for the ACL. Must be unique within the VPC. Changing this forces replacement.
- `vpc` (String) Slug of the `zcp_vpc` this ACL belongs to. Changing this forces replacement.

### Optional

- `description` (String) Human-readable description. Changing this forces replacement.

### Read-Only

- `id` (String) Network ACL ID (UUID).
