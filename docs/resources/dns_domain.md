---
page_title: "zcp_dns_domain Resource"
description: |-
  Create and manage ZCP DNS domains (zones).
---

# zcp_dns_domain

Manages a ZCP DNS domain (zone) on the platform's managed DNS. Add records with
`zcp_dns_record`. The API has no update endpoint for domains, so every change
forces replacement.

DNS is account-level: domains are scoped to a project, and the platform serves
them from a dedicated DNS provider with a single `default` region. Both values
default correctly, so most configs only set `name`.

## Example Usage

```terraform
resource "zcp_dns_domain" "example" {
  name = "example.com"
}

resource "zcp_dns_record" "www" {
  domain  = zcp_dns_domain.example.id
  name    = "www"
  type    = "A"
  content = zcp_instance.web.public_ip
  ttl     = 3600
}
```

## Import

Import using `<slug>[/<region>/<cloud_provider>/<project>]`. Omit the optional
fields when the config relies on the defaults:

```shell
terraform import zcp_dns_domain.example example-com
```

## Schema

### Required

- `name` (String) Fully qualified domain name (e.g. `example.com`). Changing
  this forces replacement.

### Optional

- `cloud_provider` (String) Cloud provider slug. Defaults to `dns`, the
  dedicated DNS provider; compute providers are invalid here. Changing this
  forces replacement.
- `region` (String) Region slug. Defaults to `default`, the single DNS region;
  compute regions are invalid here. Changing this forces replacement.
- `dns_provider` (String) DNS backend for the zone. Defaults to `PowerDNS`.
  Changing this forces replacement.
- `project` (String) Project slug. Inherits from the provider `default_project`
  if omitted. Changing this forces replacement.
- `timeouts` (Block) Configurable `create` and `delete` timeouts.

### Read-Only

- `id` (String) DNS domain slug.
- `status` (Boolean) Whether the domain is active.
