# Full VPC networking stack: VPC -> ACL -> rules -> subnet -> attach ACL.
# Each object is a first-class resource wired by reference (the AWS/Azure model).

data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_vpc" "main" {
  name             = "main-vpc"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  cidr             = "10.1.0.1"
  size             = "22"
  type             = "Vpc"
  billing_cycle    = "hourly"
  plan             = "virtual-private-cloud-vpc" # yow-1 VPC plan
  storage_category = "nvme"
}

# A network ACL inside the VPC.
resource "zcp_network_acl" "web" {
  name = "web-acl"
  vpc  = zcp_vpc.main.id
}

# Allow inbound HTTPS.
resource "zcp_network_acl_rule" "https_in" {
  vpc          = zcp_vpc.main.id
  acl          = zcp_network_acl.web.id
  number       = 100
  action       = "allow"
  traffic_type = "ingress"
  protocol     = "tcp"
  cidr_list    = "0.0.0.0/0"
  start_port   = 443
  end_port     = 443
}

# Allow all egress.
resource "zcp_network_acl_rule" "egress_all" {
  vpc          = zcp_vpc.main.id
  acl          = zcp_network_acl.web.id
  number       = 200
  action       = "allow"
  traffic_type = "egress"
  protocol     = "all"
  cidr_list    = "0.0.0.0/0"
}

# A subnet (tier) in the VPC with the ACL attached.
resource "zcp_network" "web_tier" {
  name           = "web-tier"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  vpc            = zcp_vpc.main.id
  gateway        = "10.1.1.1"
  netmask        = "255.255.255.0"
  acl            = zcp_network_acl.web.id
  billing_cycle  = "hourly"
}
