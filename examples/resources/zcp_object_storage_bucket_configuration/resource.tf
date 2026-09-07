resource "zcp_object_storage_bucket_versioning" "assets" {
  object_storage = zcp_object_storage.assets.id
  bucket         = zcp_object_storage_bucket.assets.id
  enabled        = true
}

resource "zcp_object_storage_bucket_tagging" "assets" {
  object_storage = zcp_object_storage.assets.id
  bucket         = zcp_object_storage_bucket.assets.id
  tags = {
    environment = "production"
  }
}

resource "zcp_object_storage_bucket_lifecycle" "assets" {
  object_storage = zcp_object_storage.assets.id
  bucket         = zcp_object_storage_bucket.assets.id
  days           = 30
}

resource "zcp_object_storage_bucket_cors" "assets" {
  object_storage  = zcp_object_storage.assets.id
  bucket          = zcp_object_storage_bucket.assets.id
  allowed_origins = ["https://example.com"]
  allowed_methods = ["GET"]
}
