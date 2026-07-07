---
page_title: "zcp_ssh_key Resource"
description: |-
  Create and manage ZCP SSH keys.
---

# zcp_ssh_key

Manages a ZCP SSH public key. SSH keys can be attached to virtual machines at
creation time to enable key-based authentication.

Because the ZCP API provides no update endpoint for SSH keys, all attributes are
immutable. Changing any of `name`, `public_key`, or `project` forces
replacement.

## Example Usage

```terraform
resource "zcp_ssh_key" "deploy" {
  name       = "deploy-key"
  public_key = file("~/.ssh/id_ed25519.pub")
}
```

## Import

Import using `<slug>/<region>[/<project>]`. Omit `<project>` when the config
relies on the provider `default_project`:

```shell
terraform import zcp_ssh_key.deploy deploy-key-a1b2/yow-1
```

The API returns only a normalized form of the key material, so import leaves
`public_key` unset. The first apply after import records the configured
`public_key` in state without replacing the key. Later changes to it force
replacement as usual.

## Schema

### Required

- `name` (String) Display name for the SSH key. Changing this forces
  replacement.
- `public_key` (String, Sensitive) OpenSSH public key material (e.g. the
  contents of `~/.ssh/id_ed25519.pub`). Changing this forces replacement.

### Optional

- `project` (String) Project slug. Inherits from the provider `default_project`
  if omitted. Changing this forces replacement.
- `timeouts` (Block) Configurable `create` and `delete` timeouts.

### Read-Only

- `id` (String) SSH key slug (unique identifier).
- `created_at` (String) Creation timestamp.
