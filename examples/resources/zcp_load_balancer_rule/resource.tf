resource "zcp_load_balancer_rule" "http" {
  load_balancer = zcp_load_balancer.web.id
  name          = "http"
  public_port   = "80"
  private_port  = "8080"
  algorithm     = "roundrobin"
}
