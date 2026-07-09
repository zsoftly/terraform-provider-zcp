---
page_title: "zcp_dns_record Resource"
description: |-
  Create and manage DNS records in a ZCP DNS domain.
---

# zcp_dns_record

Manages a DNS record set in a `zcp_dns_domain`. The DNS backend models records
as record sets identified by name and type, so declare one resource per
name/type pair. Every change forces replacement.

`name` is the relative label (e.g. `www`); the backend appends the zone.
`content` is write-only: the API does not return record contents in a comparable
form, so out-of-band content changes are not detected as drift.

## Example Usage

```terraform
resource "zcp_dns_domain" "example" {
  name = "example.com"
}

# name is the relative label; the backend appends the zone.
resource "zcp_dns_record" "www" {
  domain  = zcp_dns_domain.example.id
  name    = "www"
  type    = "A"
  content = "203.0.113.10"
  ttl     = 3600
}

# Apex record: use "@" for the zone itself.
resource "zcp_dns_record" "spf" {
  domain  = zcp_dns_domain.example.id
  name    = "@"
  type    = "TXT"
  content = "\"v=spf1 -all\""
  ttl     = 3600
}
```

## Import

Import using `<domain-slug>/<type>/<relative-name>`. `content` cannot be
imported; set it in config first to avoid a replacement plan:

```shell
terraform import zcp_dns_record.www example-com/A/www
```

## Schema

### Required

- `domain` (String) Parent DNS domain slug. Changing this forces replacement.
- `name` (String) Relative record name (e.g. `www`), or `@` for the zone apex.
  The zone is appended by the backend. Changing this forces replacement.
- `type` (String) Record type: `A`, `AAAA`, `CNAME`, `MX`, `TXT`, `NS`, or
  `SRV`. Changing this forces replacement.
- `content` (String) Record content (e.g. an IPv4 address for `A`). Write-only.
  Changing this forces replacement.
- `ttl` (Number) Time to live in seconds (e.g. `3600`). Changing this forces
  replacement.

### Optional

- `timeouts` (Block) Configurable `create` and `delete` timeouts.

### Read-Only

- `id` (String) Synthetic record identifier (`<type>/<fqdn>`).
- `fqdn` (String) Fully qualified record name as stored by the backend (e.g.
  `www.example.com.`).
