resource "zcp_vpc_vpn_gateway" "main" {
  vpc = zcp_vpc.main.id
}
