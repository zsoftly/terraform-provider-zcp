resource "zcp_vm_snapshot" "pre_upgrade" {
  virtual_machine = zcp_instance.web.id
  name            = "pre-upgrade"
  billing_cycle   = "monthly"
  plan            = "vm-snapshot-yow"
  cloud_provider  = data.zcp_region.yow.cloud_provider
  region          = data.zcp_region.yow.slug
}
