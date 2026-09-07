---
page_title: "zcp_object_storage_bucket Resource"
description: |-
  Create and manage buckets in a ZCP object storage store.
---

# zcp_object_storage_bucket

Manages a bucket in a `zcp_object_storage` store.

~> Bucket contents remain outside this resource. Manage bucket versioning,
policy, tags, lifecycle, and CORS with the matching bucket configuration
resources. Those resources use the store's S3-compatible gateway, obtain gateway
credentials internally, and do not expose them as resource attributes. They do
not manage object upload, download, copy, move, presigned URLs, or bucket
encryption.

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

- `object_storage` (String) Parent object storage slug. Changing this forces
  replacement.
- `name` (String) Bucket name. Changing this forces replacement.

### Optional

- `timeouts` (Block) Configurable `create` and `delete` timeouts.

### Read-Only

- `id` (String) Bucket slug.
- `status` (String) Current bucket status.
