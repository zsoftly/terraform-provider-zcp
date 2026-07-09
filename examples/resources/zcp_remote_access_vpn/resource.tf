resource "zcp_remote_access_vpn" "office" {
  ip_address = zcp_ip_address.vpn.id
}
