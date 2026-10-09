---
page_title: "zcp_object_storage_key Resource"
description: |-
  Create and revoke ZCP object storage S3 access keys.
---

# zcp_object_storage_key

Manages an S3 access key for a `zcp_object_storage` store.

The API returns the plaintext secret only while the key's visibility window is
open. Terraform stores `api_secret` in state after create and preserves that
state value on later refreshes when the API no longer returns the secret. Treat
Terraform state for this resource as sensitive.

The platform keeps one or two active keys per store. Create a replacement key,
update consumers to use it, then revoke the old key in a later apply. This
resource does not revoke another key during rotation.

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
  storage_category = "hdd-storage"
  size_gb          = 100
}

resource "zcp_object_storage_key" "assets" {
  object_storage = zcp_object_storage.assets.id
}
```

Bucket configuration resources such as `zcp_object_storage_bucket_versioning`,
`zcp_object_storage_bucket_policy`, `zcp_object_storage_bucket_tagging`,
`zcp_object_storage_bucket_lifecycle`, and `zcp_object_storage_bucket_cors` use
the S3 gateway. Export the key values for the provider process when you manage
those settings:

```shell
export ZCP_S3_ACCESS_KEY="<api_key>"
export ZCP_S3_SECRET_KEY="<api_secret>"
```

## Import

Import using `<object_storage>/<key_id>`:

```shell
terraform import zcp_object_storage_key.assets assets-x1/key-id
```

Imported keys cannot recover an expired plaintext secret from the API. Configure
dependent S3 providers with a secret captured when the key was created, or
replace the imported key to generate a new secret.

## Schema

### Required

- `object_storage` (String) Parent object storage slug. Changing this forces
  replacement.

### Optional

- `timeouts` (Block) Configurable `create` and `delete` timeouts.

### Read-Only

- `id` (String) Object storage key ID.
- `api_key` (String, Sensitive) S3 access key.
- `api_secret` (String, Sensitive) S3 secret key. Stored in Terraform state
  after create and preserved after API secret visibility expires.
- `status` (String) Key status.
- `is_primary` (Boolean) Whether this is the store's primary key.
- `secret_visible_until` (String) RFC3339 timestamp until which the API may
  return the plaintext secret.
- `created_at` (String) Creation timestamp.
- `updated_at` (String) Last update timestamp.
