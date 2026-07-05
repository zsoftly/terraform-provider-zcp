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
  name             = "qa-acl-vpc"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  cidr             = "10.7.0.1"
  size             = "22"
  type             = "Vpc"
  billing_cycle    = "hourly"
  plan             = "virtual-private-cloud-vpc"
  storage_category = "nvme"
}

resource "zcp_network_acl" "web" {
  name = "qa-web-acl"
  vpc  = zcp_vpc.main.id
}

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

resource "zcp_network_acl_rule" "egress_all" {
  vpc          = zcp_vpc.main.id
  acl          = zcp_network_acl.web.id
  number       = 200
  action       = "allow"
  traffic_type = "egress"
  protocol     = "all"
  cidr_list    = "0.0.0.0/0"
}

resource "zcp_network" "web_tier" {
  name           = "qa-web-tier"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  vpc            = zcp_vpc.main.id
  gateway        = "10.7.1.1"
  netmask        = "255.255.255.0"
  acl            = zcp_network_acl.web.id
  billing_cycle  = "hourly"
}

output "vpc_id" { value = zcp_vpc.main.id }
output "acl_id" { value = zcp_network_acl.web.id }
output "rule_https_id" { value = zcp_network_acl_rule.https_in.id }
output "tier_id" { value = zcp_network.web_tier.id }
