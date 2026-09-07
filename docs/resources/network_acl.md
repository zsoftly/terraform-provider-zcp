---
page_title: "zcp_network_acl Resource"
description: |-
  Create and manage ZCP Network ACLs inside a VPC.
---

# zcp_network_acl

Manages a ZCP Network ACL, a stateful allow/deny rule set that lives inside a
VPC. Add rules with `zcp_network_acl_rule` and attach the ACL to a subnet with
the `acl` argument on `zcp_network`. This mirrors `aws_network_acl` /
`azurerm_network_security_group`. The ACL is a container, rules are separate
resources, and you wire them together by reference.

~> **Stateful, not stateless:** the platform tracks connections at the ACL and
automatically accepts replies to traffic permitted by an allow rule. Ingress and
egress rules do not correlate, so an inbound listener still needs its own
explicit ingress rule.

An egress deny-all rule does not block replies to a connection an ingress rule
already allowed. Outbound connections need no ephemeral-port catch-all rule.

Connection tracking applies to both directions independently. An allowed
connection in either direction gets its own return traffic accepted
automatically, and that acceptance never becomes an implicit rule in the other
direction.

Operators who added an ingress rule allowing `tcp`/`udp` on `1024-65535` from
`0.0.0.0/0` to allow reply traffic under the old "stateless" guidance can remove
it. Connection tracking makes the rule unnecessary and exposes every high port
on the tier to the internet. This behavior is not tied to a particular network
offering. The provider has verified it against ZCP's current platform release
(2026-09).

~> **No update endpoint:** the API cannot update an ACL in place, so changing
`name`, `vpc`, or `description` forces replacement (which recreates its rules
and reattaches them via the dependency graph).

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

Import using `<vpc-slug>/<acl-id>` (the VPC slug is needed for every ACL API
call):

```shell
terraform import zcp_network_acl.web main-vpc/<acl-id>
```

## Schema

### Required

- `name` (String) Display name for the ACL. Must be unique within the VPC.
  Changing this forces replacement.
- `vpc` (String) Slug of the `zcp_vpc` this ACL belongs to. Changing this forces
  replacement.

### Optional

- `description` (String) Human-readable description. Changing this forces
  replacement.

### Read-Only

- `id` (String) Network ACL ID (UUID).
