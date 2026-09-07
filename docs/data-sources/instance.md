---
page_title: "zcp_instance Data Source"
description: |-
  Look up an existing ZCP instance by slug.
---

# zcp_instance (Data Source)

Looks up an existing instance by slug, for example to attach resources to an
instance created outside Terraform.

The attached-volume lookup behind `root_volume` and `volumes` retrieves every
page of block storage results. `region` and `project` narrow the lookup. Results
are complete within that scope.

## Example Usage

```terraform
data "zcp_instance" "web" {
  slug = "vm1-web"
}

output "root_volume" {
  value = data.zcp_instance.web.root_volume
}
```

Use `root_volume` to point a `zcp_volume_backup` at an instance's boot disk. A
hardcoded volume slug breaks when the instance is rebuilt. `root_volume` stays
correct.

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

data "zcp_instance" "web" {
  slug   = "vm1-web"
  region = data.zcp_region.yow.slug
}

resource "zcp_volume_backup" "web_root" {
  volume         = data.zcp_instance.web.root_volume
  interval       = "dailyAt"
  at             = 1
  plan           = "backup-yow"
  billing_cycle  = "hourly"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
}
```

## Schema

### Required

- `slug` (String) Instance slug.

### Optional

- `region` (String) Region slug to scope the attached-volume lookup (e.g.
  `yow-1`). If omitted, volumes are listed across all regions and filtered to
  this instance.
- `project` (String) Project slug to scope the attached-volume lookup. Inherits
  from the provider `default_project` if omitted.

### Read-Only

- `id` (String) Instance slug (same as `slug`).
- `name` (String) Instance display name.
- `state` (String) Current power state.
- `private_ip` (String) Private IP address.
- `public_ip` (String) Public IP address, if assigned.
- `root_volume` (String) Slug of the root volume attached to this instance.
  Empty if no root volume is found.
- `volumes` (List of String) Slugs of all volumes attached to this instance,
  root volume first.
