---
page_title: "zcp_object_storage_bucket Resource"
description: |-
  Create and manage buckets in a ZCP object storage store.
---

# zcp_object_storage_bucket

Manages a bucket in a `zcp_object_storage` store.

~> Bucket contents, policies, versioning, and lifecycle settings are managed via the S3 API (e.g. the AWS or minio providers pointed at the store's endpoint with its `api_key`/`api_secret`), not by this resource.

## Example Usage

```terraform
resource "zcp_object_storage_bucket" "media" {
  object_storage = zcp_object_storage.assets.id
  name           = "media"
}
```

## Import

Import using `<store-slug>/<bucket-slug>`:

```shell
terraform import zcp_object_storage_bucket.media assets-x1/media-b1
```

## Schema

### Required

- `object_storage` (String) Parent object storage slug. Changing this forces replacement.
- `name` (String) Bucket name. Changing this forces replacement.

### Read-Only

- `id` (String) Bucket slug.
- `status` (String) Current bucket status.
