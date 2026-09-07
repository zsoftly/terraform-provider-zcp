data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_network" "prod" {
  name           = "prod-network"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  network_plan   = "inet-yow"
  billing_cycle  = "hourly"
}

resource "zcp_instance" "web" {
  name             = "web-01"
  template         = "ubuntu-2604-lts-1"
  plan             = "ca2sl"
  billing_cycle    = "hourly"
  network          = zcp_network.prod.id
  storage_category = "nvme"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
}

resource "zcp_vm_backup" "web_daily" {
  virtual_machine = zcp_instance.web.id
  interval        = "dailyAt"
  at              = 2
  plan            = "backup-yow"
  billing_cycle   = "hourly"
  cloud_provider  = data.zcp_region.yow.cloud_provider
  region          = data.zcp_region.yow.slug
}
