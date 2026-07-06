---
page_title: "zcp_dns_domain Resource"
description: |-
  Create and manage ZCP DNS domains (zones).
---

# zcp_dns_domain

Manages a ZCP DNS domain (zone) on the platform's managed DNS. Add records with `zcp_dns_record`. The API has no update endpoint for domains, so every change forces replacement.

## Example Usage

```terraform
# DNS domains live in the account-level `default` region.
data "zcp_region" "default" {
  slug = "default"
}

resource "zcp_dns_domain" "example" {
  name           = "example.com"
  cloud_provider = data.zcp_region.default.cloud_provider
  region         = data.zcp_region.default.slug
}
```

## Import

Import using `<slug>/<region>/<cloud_provider>[/<project>]`. Omit `<project>` when the config relies on the provider `default_project`:

```shell
terraform import zcp_dns_domain.example example-com/default/nimbo
```

## Schema

### Required

- `name` (String) Fully qualified domain name (e.g. `example.com`). Changing this forces replacement.
- `cloud_provider` (String) Cloud provider slug. Changing this forces replacement.
- `region` (String) Region slug. DNS domains live in the account-level `default` region. Changing this forces replacement.

### Optional

- `dns_provider` (String) DNS provider backing the zone. Defaults to `PowerDNS`. Changing this forces replacement.
- `project` (String) Project slug. Inherits from the provider `default_project` if omitted. Changing this forces replacement.

### Read-Only

- `id` (String) DNS domain slug.
- `status` (Boolean) Whether the domain is active.
