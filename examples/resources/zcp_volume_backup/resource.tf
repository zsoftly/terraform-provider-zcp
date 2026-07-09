resource "zcp_volume_backup" "data_daily" {
  volume         = zcp_volume.data.id
  interval       = "dailyAt"
  at             = 1
  plan           = "backup-yow"
  billing_cycle  = "hourly"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
}
