resource "zcp_load_balancer_attachment" "web_vm1" {
  load_balancer   = zcp_load_balancer.web.id
  rule            = zcp_load_balancer.web.rule_id
  virtual_machine = zcp_instance.web.id
  cloud_provider  = data.zcp_region.yow.cloud_provider
  region          = data.zcp_region.yow.slug
}
