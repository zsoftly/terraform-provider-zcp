# Acquire a public IP and associate it to an instance (the AWS EIP model):
#   zcp_ip_address      = acquire/own the IP (aws_eip)
#   zcp_ip_association  = attach it to an instance via static NAT (aws_eip_association)
#
# A public IP is acquired within a VPC (which must already have a subnet) or a
# standalone network. The association's `network` is the network the instance is
# on, so it works for VPC tiers and standalone networks alike.

data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_vpc" "main" {
  name             = "main-vpc"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  cidr             = "10.1.0.1"
  size             = "22"
  billing_cycle    = "hourly"
  plan             = "virtual-private-cloud-vpc"
  storage_category = "nvme"
}

resource "zcp_network" "tier" {
  name           = "app-tier"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  vpc            = zcp_vpc.main.id
  gateway        = "10.1.1.1"
  netmask        = "255.255.255.0"
  billing_cycle  = "hourly"
}

# Acquire and own a static public IP in the VPC. The VPC must already have a
# network before an IP can be acquired, so depend on the tier.
resource "zcp_ip_address" "eip" {
  vpc           = zcp_vpc.main.id
  plan          = "ipv4-yow"
  billing_cycle = "hourly"
  depends_on    = [zcp_network.tier]
}

resource "zcp_instance" "web" {
  name             = "web-01"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  template         = "ubuntu-2404-lts"
  plan             = "ci1xs"
  billing_cycle    = "hourly"
  network          = zcp_network.tier.id
  storage_category = "nvme"
}

# Associate the owned IP with the instance (static NAT).
resource "zcp_ip_association" "web" {
  ip_address      = zcp_ip_address.eip.id
  virtual_machine = zcp_instance.web.id
  network         = zcp_network.tier.id
}
