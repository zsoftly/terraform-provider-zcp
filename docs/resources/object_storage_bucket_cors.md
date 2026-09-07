---
page_title: "zcp_object_storage_bucket_cors Resource"
description: |-
  Manage CORS for a ZCP object storage bucket.
---

# zcp_object_storage_bucket_cors

Manages one CORS rule through the object storage gateway. The SDK resolves the
gateway endpoint from the selected object-storage instance. This resource owns
the CORS configuration and requires the bucket to have exactly one CORS rule.

```terraform
resource "zcp_object_storage_bucket_cors" "media" {
  object_storage  = zcp_object_storage.assets.id
  bucket          = zcp_object_storage_bucket.media.id
  allowed_origins = ["https://example.com"]
  allowed_methods = ["GET"]
}
```

Import using `<store-slug>/<bucket-slug>`.

## Argument Reference

- `object_storage` (String, required) Store slug.
- `bucket` (String, required) Bucket slug.
- `allowed_origins` and `allowed_methods` (List of String, required) Nonempty
  CORS origin and method lists.
- `allowed_headers` (List of String, optional) Allowed request headers.
- `max_age_seconds` (Number, optional) Preflight cache duration. Defaults to
  zero.
