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

# Allow outbound HTTPS from the network to anywhere.
resource "zcp_egress_rule" "https_out" {
  network    = zcp_network.prod.id
  protocol   = "tcp"
  cidr       = "0.0.0.0/0"
  start_port = "443"
  end_port   = "443"
}

# Allow outbound DNS.
resource "zcp_egress_rule" "dns_out" {
  network    = zcp_network.prod.id
  protocol   = "udp"
  cidr       = "0.0.0.0/0"
  start_port = "53"
  end_port   = "53"
}
