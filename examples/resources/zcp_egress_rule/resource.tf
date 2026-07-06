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
