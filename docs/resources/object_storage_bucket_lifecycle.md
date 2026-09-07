---
page_title: "zcp_object_storage_bucket_lifecycle Resource"
description: |-
  Manage one lifecycle rule for a ZCP object storage bucket.
---

# zcp_object_storage_bucket_lifecycle

Manages one expiry rule through the object storage gateway. Set any duration to
zero or omit it to leave that action disabled.

```terraform
resource "zcp_object_storage_bucket_lifecycle" "uploads" {
  object_storage = zcp_object_storage.assets.id
  bucket         = zcp_object_storage_bucket.media.id
  prefix         = "uploads/"
  days           = 30
}
```

Import using `<store-slug>/<bucket-slug>`.

## Argument Reference

- `object_storage` (String, required) Store slug.
- `bucket` (String, required) Bucket slug.
- `prefix` (String, optional) Key prefix. Defaults to an empty prefix.
- `days`, `noncurrent_days`, and `abort_incomplete_multipart_upload_days`
  (Number, optional) Lifecycle durations. Set at least one to a positive value.
