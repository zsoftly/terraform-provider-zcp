---
page_title: "zcp_ssh_key Data Source"
description: |-
  Look up an existing ZCP SSH key by slug or name.
---

# zcp_ssh_key (Data Source)

Looks up an existing SSH key by slug or name, for example to reference a key uploaded through the console.

## Example Usage

```terraform
data "zcp_ssh_key" "deploy" {
  name = "deploy"
}

resource "zcp_instance" "web" {
  # ...
  ssh_key = data.zcp_ssh_key.deploy.id
}
```

## Schema

### Optional

- `slug` (String) SSH key slug. Exactly one of `slug` or `name` must be set.
- `name` (String) SSH key display name. Exactly one of `slug` or `name` must be set.

### Read-Only

- `id` (String) SSH key slug.
- `public_key` (String) OpenSSH public key material.
- `created_at` (String) Creation timestamp.
