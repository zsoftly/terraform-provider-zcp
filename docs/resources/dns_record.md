---
page_title: "zcp_dns_record Resource"
description: |-
  Create and manage DNS records in a ZCP DNS domain.
---

# zcp_dns_record

Manages a DNS record set in a `zcp_dns_domain`. The DNS backend models records as record sets identified by name and type, so declare one resource per name/type pair. Every change forces replacement.

`name` is the relative label (e.g. `www`); the backend appends the zone. `content` is write-only: the API does not return record contents in a comparable form, so out-of-band content changes are not detected as drift.

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

resource "zcp_dns_record" "www" {
  domain  = zcp_dns_domain.example.id
  name    = "www"
  type    = "A"
  content = zcp_instance.web.public_ip
  ttl     = 3600
}
```

## Import

Import using `<domain-slug>/<type>/<relative-name>`. `content` cannot be imported; set it in config first to avoid a replacement plan:

```shell
terraform import zcp_dns_record.www example-com/A/www
```

## Schema

### Required

- `domain` (String) Parent DNS domain slug. Changing this forces replacement.
- `name` (String) Relative record name (e.g. `www`). The zone is appended by the backend. Changing this forces replacement.
- `type` (String) Record type: `A`, `AAAA`, `CNAME`, `MX`, `TXT`, `NS`, or `SRV`. Changing this forces replacement.
- `content` (String) Record content (e.g. an IPv4 address for `A`). Write-only. Changing this forces replacement.
- `ttl` (Number) Time to live in seconds (e.g. `3600`). Changing this forces replacement.

### Read-Only

- `id` (String) Synthetic record identifier (`<type>/<fqdn>`).
- `fqdn` (String) Fully qualified record name as stored by the backend (e.g. `www.example.com.`).
