# terraform-provider-zcp v1.2.0 Release Notes

Provider v1.2.0 adds `zcp_object_storage_bucket_encryption`, which manages a
bucket's default server-side encryption as Terraform or OpenTofu state. New
uploads to the bucket are encrypted with SSE-S3 or with a service-managed
SSE-KMS key. Existing objects and versions stay unchanged.

Set `ZCP_S3_ACCESS_KEY` and `ZCP_S3_SECRET_KEY` to an active key for the store
before applying. The other bucket configuration resources use the same
credentials, and the resource does not store them in state.

For SSE-S3, omit `kms_key_id`:

```hcl
resource "zcp_object_storage_bucket_encryption" "assets" {
  object_storage = zcp_object_storage.assets.id
  bucket         = zcp_object_storage_bucket.assets.id
  algorithm      = "AES256"
}
```

For SSE-KMS, set `algorithm = "aws:kms"` and supply the service-managed key ID:

```hcl
resource "zcp_object_storage_bucket_encryption" "assets" {
  object_storage = zcp_object_storage.assets.id
  bucket         = zcp_object_storage_bucket.assets.id
  algorithm      = "aws:kms"
  kms_key_id     = var.kms_key_id
}
```

Import with `<store-slug>/<bucket-slug>`. If someone removes the default outside
Terraform or OpenTofu, the next plan recreates it.

```bash
terraform import zcp_object_storage_bucket_encryption.assets assets-x1/media-b1
```

Destroying the resource removes the default without decrypting or deleting any
data. Encryption depends on the region's key management service, so check
availability with ZCP support before you rely on it.

A copy made without encryption settings does not pick up the bucket default.
For encrypted copies, use `zcp object-storage object copy --sse`, or a multipart
copy in other S3 clients. The service refuses a single-request copy that asks
for encryption. The provider builds against zcp-cli v1.2.0.

## Upgrade

Set the provider constraint to `~> 1.2.0`, then run `terraform init -upgrade` or
`tofu init -upgrade`. Review the plan before applying it.
