# An ACL rule is an independent resource (like aws_network_acl_rule). Manage many
# rules with for_each, each referencing the parent zcp_network_acl.

resource "zcp_network_acl_rule" "rules" {
  for_each = {
    https = { number = 100, traffic_type = "ingress", protocol = "tcp", start_port = 443, end_port = 443 }
    ssh   = { number = 110, traffic_type = "ingress", protocol = "tcp", start_port = 22, end_port = 22 }
  }

  vpc          = zcp_vpc.main.id
  acl          = zcp_network_acl.web.id
  number       = each.value.number
  action       = "allow"
  traffic_type = each.value.traffic_type
  protocol     = each.value.protocol
  cidr_list    = "0.0.0.0/0"
  start_port   = each.value.start_port
  end_port     = each.value.end_port
}
