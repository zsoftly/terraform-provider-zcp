terraform {
  required_providers {
    zcp = { source = "registry.terraform.io/zsoftly/zcp" }
  }
}

variable "zcp_token" {
  type      = string
  sensitive = true
}

provider "zcp" {
  api_url         = "https://api.zcp.zsoftly.ca/api"
  bearer_token    = var.zcp_token
  default_project = "default-9"
}

data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_vpc" "main" {
  name             = "qa-eip-vpc"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  cidr             = "10.8.0.1"
  size             = "22"
  type             = "Vpc"
  billing_cycle    = "hourly"
  plan             = "virtual-private-cloud-vpc"
  storage_category = "nvme"
}

resource "zcp_network" "tier" {
  name           = "qa-eip-tier"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  vpc            = zcp_vpc.main.id
  gateway        = "10.8.1.1"
  netmask        = "255.255.255.0"
  billing_cycle  = "hourly"
}

# Acquire and own a static public IP in the VPC. The VPC must already have a
# network, so depend on the tier.
resource "zcp_ip_address" "eip" {
  vpc           = zcp_vpc.main.id
  plan          = "ipv4-yow"
  billing_cycle = "hourly"
  depends_on    = [zcp_network.tier]
}

resource "zcp_instance" "web" {
  name             = "qa-eip-instance"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  template         = "ubuntu-2404-lts"
  plan             = "ci1xs"
  billing_cycle    = "hourly"
  network          = zcp_network.tier.id
  storage_category = "nvme"

  tags = {
    date   = "2026-06-22"
    ticket = "TICKET-13"
  }

  timeouts {
    create = "30m"
  }
}

# Associate the owned IP with the instance (static NAT).
resource "zcp_ip_association" "web" {
  ip_address      = zcp_ip_address.eip.id
  virtual_machine = zcp_instance.web.id
  network         = zcp_network.tier.id
}

output "vpc_id" { value = zcp_vpc.main.id }
output "tier_id" { value = zcp_network.tier.id }
output "public_ip" { value = zcp_ip_address.eip.ip_address }
output "instance_id" { value = zcp_instance.web.id }
output "instance_state" { value = zcp_instance.web.state }
output "instance_private_ip" { value = zcp_instance.web.private_ip }
output "association_id" { value = zcp_ip_association.web.id }
