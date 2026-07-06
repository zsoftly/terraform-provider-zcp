resource "zcp_object_storage_bucket" "media" {
  object_storage = zcp_object_storage.assets.id
  name           = "media"
}
