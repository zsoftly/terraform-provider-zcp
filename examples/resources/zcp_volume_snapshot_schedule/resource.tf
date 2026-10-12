data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_volume_snapshot_schedule" "data" {
  volume    = "data-01"
  interval  = "dailyAt"
  at        = "02:00"
  timezone  = "America/Toronto"
  retention = 7
  region    = data.zcp_region.yow.slug
}
