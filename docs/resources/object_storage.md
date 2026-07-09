---
page_title: "zcp_object_storage Resource"
description: |-
  Create and manage ZCP object storage stores.
---

# zcp_object_storage

Manages a ZCP object storage store (S3-compatible). Create buckets with
`zcp_object_storage_bucket`. `size_gb` resizes the store in place; every other
change forces replacement.

The store's S3 credentials are exported as the sensitive attributes `api_key`
and `api_secret`.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_object_storage" "assets" {
  name             = "assets"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  billing_cycle    = "hourly"
  storage_category = "nvme"
  size_gb          = 100
}
```

## Import

Import using
`<slug>/<cloud_provider>/<region>/<billing_cycle>/<storage_category>[/<plan>/<project>]`:

```shell
terraform import zcp_object_storage.assets assets-x1/zsoftly/yow-1/hourly/nvme
```

## Schema

### Required

- `name` (String) Display name. Changing this forces replacement.
- `cloud_provider` (String) Cloud provider slug. Changing this forces
  replacement.
- `region` (String) Region slug (e.g. `yow-1`). Changing this forces
  replacement.
- `billing_cycle` (String) Billing cycle (e.g. `hourly`, `monthly`). Changing
  this forces replacement.
- `storage_category` (String) Storage category slug (region-specific, e.g.
  `nvme`, `pro-nvme`). Changing this forces replacement.

### Optional

- `project` (String) Project slug. Inherits from the provider `default_project`
  if omitted. Changing this forces replacement.
- `plan` (String) Catalogue plan slug. Exactly one of `plan` or `size_gb` must
  be set. Changing this forces replacement.
- `size_gb` (Number) Custom store size in GB. Exactly one of `plan` or `size_gb`
  must be set. Increasing it resizes the store in place.
- `timeouts` (Block) Configurable `create`, `update`, and `delete` timeouts.

### Read-Only

- `id` (String) Object storage slug.
- `status` (String) Current status.
- `size` (Number) Provisioned size in GB as reported by the API.
- `api_key` (String, Sensitive) S3 access key.
- `api_secret` (String, Sensitive) S3 secret key.
