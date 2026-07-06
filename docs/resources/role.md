---
page_title: "zcp_role Resource"
description: |-
  Create and manage ZCP roles for sub-users.
---

# zcp_role

Manages a ZCP role. `permissions` is the full desired set; updates replace the role's existing permissions. Use the `zcp_permissions` data source to discover permission slugs.

## Example Usage

```terraform
data "zcp_permissions" "compute" {
  category = "compute"
}

resource "zcp_role" "ops" {
  name        = "ops"
  description = "Operations team"
  permissions = data.zcp_permissions.compute.permissions[*].slug
}
```

## Import

Import using the role slug:

```shell
terraform import zcp_role.ops ops-r1
```

## Schema

### Required

- `name` (String) Role display name. Updated in place.
- `permissions` (Set of String) Permission slugs granted by the role. Must contain at least one permission. The set replaces the role's permissions on update.

### Optional

- `description` (String) Human-readable description. Updated in place; removing it clears the description.

### Read-Only

- `id` (String) Role slug.
