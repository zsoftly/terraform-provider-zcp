data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_vpn_customer_gateway" "office" {
  name           = "office"
  gateway        = "203.0.113.99"
  cidr_list      = "192.168.10.0/24"
  ipsec_psk      = var.office_psk
  ike_policy     = "aes128-sha1-dh5"
  esp_policy     = "aes128-sha1"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
}
