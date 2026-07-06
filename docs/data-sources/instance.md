---
page_title: "zcp_instance Data Source"
description: |-
  Look up an existing ZCP instance by slug.
---

# zcp_instance (Data Source)

Looks up an existing instance by slug, for example to attach resources to an instance created outside Terraform.

## Example Usage

```terraform
data "zcp_instance" "legacy" {
  slug = "vm1-abc"
}

resource "zcp_dns_record" "legacy" {
  domain  = zcp_dns_domain.example.id
  name    = "legacy.example.com"
  type    = "A"
  content = data.zcp_instance.legacy.public_ip
  ttl     = 3600
}
```

## Schema

### Required

- `slug` (String) Instance slug.

### Read-Only

- `id` (String) Instance slug (same as `slug`).
- `name` (String) Instance display name.
- `state` (String) Current power state.
- `private_ip` (String) Private IP address.
- `public_ip` (String) Public IP address, if assigned.
