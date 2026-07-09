data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_object_storage" "assets" {
  name             = "assets"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  billing_cycle    = "hourly"
  storage_category = "nvme"
  size_gb          = 100
}
