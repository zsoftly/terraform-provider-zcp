---
page_title: "zcp_object_storage_bucket_tagging Resource"
description: |-
  Manage tags for a ZCP object storage bucket.
---

# zcp_object_storage_bucket_tagging

Manages the complete bucket tag set through the object storage gateway.

```terraform
resource "zcp_object_storage_bucket_tagging" "media" {
  object_storage = zcp_object_storage.assets.id
  bucket         = zcp_object_storage_bucket.media.id
  tags = { environment = "production" }
}
```

Import using `<store-slug>/<bucket-slug>`.

## Argument Reference

- `object_storage` (String, required) Store slug.
- `bucket` (String, required) Bucket slug.
- `tags` (Map of String, required) Complete nonempty bucket tag set. Remove this
  resource to delete all tags.
