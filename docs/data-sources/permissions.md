---
page_title: "zcp_permissions Data Source"
description: |-
  List the ZCP permission catalog for role authorship.
---

# zcp_permissions (Data Source)

Lists the permission catalog for building `zcp_role` resources, optionally
filtered by category.

## Example Usage

```terraform
data "zcp_permissions" "compute" {
  category = "compute"
}

resource "zcp_role" "ops" {
  name        = "ops"
  permissions = data.zcp_permissions.compute.permissions[*].slug
}
```

## Schema

### Optional

- `category` (String) Return permissions in this category only.

### Read-Only

- `id` (String) Synthetic identifier.
- `permissions` (List of Object) Matching permissions, each with:
  - `id` (String) Permission ID.
  - `name` (String) Permission display name.
  - `slug` (String) Permission slug, usable in `zcp_role.permissions`.
  - `description` (String) Permission description.
  - `category` (String) Permission category.
