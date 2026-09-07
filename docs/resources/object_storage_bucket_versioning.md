---
page_title: "zcp_object_storage_bucket_versioning Resource"
description: |-
  Manage object versioning for a ZCP object storage bucket.
---

# zcp_object_storage_bucket_versioning

Manages object versioning through the object storage S3-compatible gateway.

```terraform
resource "zcp_object_storage_bucket_versioning" "media" {
  object_storage = zcp_object_storage.assets.id
  bucket         = zcp_object_storage_bucket.media.id
  enabled        = true
}
```

Import using `<store-slug>/<bucket-slug>`.

```shell
terraform import zcp_object_storage_bucket_versioning.media assets-x1/media-b1
```

## Argument Reference

- `object_storage` (String, required) Store slug.
- `bucket` (String, required) Bucket slug.
- `enabled` (Boolean, required) Enable or suspend versioning.
