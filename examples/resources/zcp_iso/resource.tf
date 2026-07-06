resource "zcp_iso" "rescue" {
  name           = "rescue-iso"
  url            = "https://mirror.example.com/rescue.iso"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  billing_cycle  = "hourly"
}
