---
page_title: "zcp_object_storage_bucket_policy Resource"
description: |-
  Manage an S3 policy for a ZCP object storage bucket.
---

# zcp_object_storage_bucket_policy

Manages the complete S3 bucket policy through the object storage gateway. This
resource owns the full bucket policy. Do not combine it with CLI visibility or
ACL commands, which update the same policy.

```terraform
resource "zcp_object_storage_bucket_policy" "media" {
  object_storage = zcp_object_storage.assets.id
  bucket         = zcp_object_storage_bucket.media.id
  policy         = jsonencode({ Version = "2012-10-17", Statement = [] })
}
```

Import using `<store-slug>/<bucket-slug>`.

## Argument Reference

- `object_storage` (String, required) Store slug.
- `bucket` (String, required) Bucket slug.
- `policy` (String, required) Valid JSON for the complete S3 bucket policy.
