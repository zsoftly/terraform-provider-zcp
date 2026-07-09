---
page_title: "zcp_sub_user Resource"
description: |-
  Create and manage ZCP sub-users (team members).
---

# zcp_sub_user

Manages a ZCP sub-user. `name`, `email`, `role`, `projects`, and `is_blocked`
update in place. The API cannot change a password after creation, so changing
`password` forces replacement.

## Example Usage

```terraform
resource "zcp_role" "ops" {
  name        = "ops"
  permissions = ["instances-view", "instances-manage"]
}

resource "zcp_sub_user" "alice" {
  name     = "Alice"
  email    = "alice@example.com"
  password = var.alice_password
  role     = zcp_role.ops.id
  projects = [zcp_project.staging.id]
}
```

## Import

Import using the sub-user ID. `password` is write-only and the API never returns
it, so the imported state has no password. The first apply after import records
the configured password in state without replacing or modifying the user. Later
password changes force replacement as usual:

```shell
terraform import zcp_sub_user.alice <user-id>
```

## Schema

### Required

- `name` (String) Display name. Updated in place.
- `email` (String) Company email address. Updated in place.
- `password` (String, Sensitive) Initial password (8+ characters with upper,
  lower, digit, and special character). Write-only. Changing this forces
  replacement.
- `role` (String) Role slug (e.g. from `zcp_role`). Updated in place.
- `projects` (List of String) Project slugs the sub-user has access to. Updated
  in place.

### Optional

- `is_blocked` (Boolean) Whether the sub-user is blocked from signing in.
  Updated in place.
- `timeouts` (Block) Configurable `create` and `delete` timeouts.

### Read-Only

- `id` (String) Sub-user ID.
- `status` (String) Current user status.
