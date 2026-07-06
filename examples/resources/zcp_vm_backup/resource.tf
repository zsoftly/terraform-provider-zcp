resource "zcp_vm_backup" "web_daily" {
  virtual_machine = zcp_instance.web.id
  interval        = "daily"
  at              = 2
  plan            = "backup-yow"
  billing_cycle   = "hourly"
  cloud_provider  = data.zcp_region.yow.cloud_provider
  region          = data.zcp_region.yow.slug
}
