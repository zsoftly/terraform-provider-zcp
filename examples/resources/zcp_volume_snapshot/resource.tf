resource "zcp_volume_snapshot" "data_nightly" {
  volume         = zcp_volume.data.id
  name           = "data-nightly"
  billing_cycle  = "hourly"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
}
