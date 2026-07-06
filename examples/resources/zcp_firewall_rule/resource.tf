resource "zcp_firewall_rule" "https_in" {
  ip_address = zcp_ip_address.web.id
  protocol   = "tcp"
  cidr_list  = "0.0.0.0/0"
  start_port = "443"
  end_port   = "443"
}
