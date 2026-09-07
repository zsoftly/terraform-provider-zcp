resource "zcp_ip_address" "web" {
  plan          = "public-ip-1"
  billing_cycle = "hourly"
  network       = zcp_network.prod.id
}

# VPC allocation: the vpc slug alone gives Terraform no ordering information
# between the tier and the IP address, so add an explicit depends_on on the
# tier.
resource "zcp_ip_address" "vpc_ip" {
  plan          = "public-ip-1"
  billing_cycle = "hourly"
  vpc           = zcp_vpc.main.id

  depends_on = [zcp_network.tier]
}
