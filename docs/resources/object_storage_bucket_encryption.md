---
page_title: "zcp_object_storage_bucket_encryption Resource"
description: |-
  Manage default server-side encryption for a ZCP object storage bucket.
---

# zcp_object_storage_bucket_encryption

Manages bucket default encryption through the existing S3-compatible gateway.
Set `ZCP_S3_ACCESS_KEY` and `ZCP_S3_SECRET_KEY` to an active key for the store
before applying. These credentials follow the existing bucket configuration
resources and are not stored in this resource's state. `kms_key_id` identifies
an operator-provisioned key. It contains no encryption key material.

Confirm regional encryption support with ZCP support before use. This resource
does not enable the gateway backend or announce availability in any region.
Unsupported gateway errors include regional setup guidance. Authorization and
invalid-key failures remain errors, with no fallback to plaintext.

## Example Usage

Use this HCL with Terraform or OpenTofu. Choose one configuration per bucket.
For SSE-S3, omit `kms_key_id`:

```terraform
resource "zcp_object_storage_bucket_encryption" "assets" {
  object_storage = zcp_object_storage.assets.id
  bucket         = zcp_object_storage_bucket.assets.id
  algorithm      = "AES256"
}
```

For SSE-KMS, set `algorithm = "aws:kms"` and supply the service-managed key ID:

```terraform
resource "zcp_object_storage_bucket_encryption" "assets" {
  object_storage = zcp_object_storage.assets.id
  bucket         = zcp_object_storage_bucket.assets.id
  algorithm      = "aws:kms"
  kms_key_id     = var.kms_key_id
}
```

See `examples/resources/zcp_object_storage_bucket_encryption/resource.tf` for a
standalone example using existing store and bucket slugs.

## Argument Reference

- `object_storage` (String, required) Store slug. Changes force replacement.
- `bucket` (String, required) Bucket slug. Changes force replacement. The
  provider resolves its S3 bucket name through the existing bucket API.
- `algorithm` (String, required) `AES256` for SSE-S3 or `aws:kms` for SSE-KMS.
- `kms_key_id` (String, optional) Nonempty, slash-free service-managed key ID.
  Required with `aws:kms` and omitted with `AES256`. This resource does not
  create keys or configure customer KMS key policies.

## Attribute Reference

- `id` (String) Composite identifier: `<store-slug>/<bucket-slug>`.

The shared bucket configuration schema includes computed fields for the other
bucket settings. They remain null for this resource.

## Import

Import using `<store-slug>/<bucket-slug>`. Refresh reads the configured
algorithm and key ID. Removing the default outside Terraform or OpenTofu
produces a plan to recreate it.

```shell
terraform import zcp_object_storage_bucket_encryption.assets assets-x1/media-b1
tofu import zcp_object_storage_bucket_encryption.assets assets-x1/media-b1
```

## Limitations

Bucket defaults affect subsequent uploads. Existing objects and versions stay
unchanged. Destroying this resource removes the default without decrypting or
deleting existing data. Retain the underlying service-managed keys needed to
read encrypted objects.

Ceph RGW `CopyObject` does not apply the destination bucket's default
encryption. Clients must send explicit SSE headers on copies requiring
encryption. With the ZCP CLI, pass `--sse AES256` or
`--sse aws:kms --sse-kms-key-id <service-key-id>` on copy and move.
[Ceph correction #72291](https://github.com/ceph/ceph/pull/72291) remains open
as of October 28, 2026. In-place re-encryption is unsupported. This resource
never copies, rewrites, or automatically re-encrypts existing objects.
