data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_vpn_user" "alice" {
  username       = "alice"
  password       = var.vpn_password
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
}
