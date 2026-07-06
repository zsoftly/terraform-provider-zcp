resource "zcp_ip_address" "web" {
  plan          = "public-ip-1"
  billing_cycle = "hourly"
  network       = zcp_network.prod.id
}
