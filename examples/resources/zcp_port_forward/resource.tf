resource "zcp_port_forward" "ssh" {
  ip_address         = zcp_ip_address.web.id
  protocol           = "tcp"
  public_start_port  = "22"
  private_start_port = "22"
  virtual_machine    = zcp_instance.web.id
}
