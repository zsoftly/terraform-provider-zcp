data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_load_balancer" "web" {
  name           = "web-lb"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  network        = zcp_network.prod.id
  billing_cycle  = "hourly"

  rule_name    = "https"
  public_port  = "443"
  private_port = "8443"
  algorithm    = "roundrobin"
}
