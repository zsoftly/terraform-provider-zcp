data "zcp_region" "r" {
  slug = "yow-1"
}

# 1. SSH key (account-level)
resource "zcp_ssh_key" "k" {
  name       = "tf-life-key"
  public_key = var.ssh_public_key
  region     = data.zcp_region.r.slug
}

# 2. Standalone isolated network
resource "zcp_network" "iso" {
  name           = "tf-life-net"
  cloud_provider = data.zcp_region.r.cloud_provider
  region         = data.zcp_region.r.slug
  network_plan   = "inet-yow"
  billing_cycle  = "hourly"
  description    = "tf lifecycle isolated network"
}

# 3. VPC
resource "zcp_vpc" "v" {
  name             = "tf-life-vpc"
  cloud_provider   = data.zcp_region.r.cloud_provider
  region           = data.zcp_region.r.slug
  cidr             = "10.7.0.1"
  size             = "24"
  type             = "Vpc"
  billing_cycle    = "hourly"
  storage_category = "nvme"
  plan             = "virtual-private-cloud-vpc-1"
  description      = "tf lifecycle vpc"
}

# 4. Chained: network created as a subnet INSIDE the VPC (references the VPC)
resource "zcp_network" "subnet" {
  name           = "tf-life-subnet"
  cloud_provider = data.zcp_region.r.cloud_provider
  region         = data.zcp_region.r.slug
  vpc            = zcp_vpc.v.id
  gateway        = "10.7.0.1"
  netmask        = "255.255.255.0"
  billing_cycle  = "hourly"
  description    = "tf lifecycle vpc subnet"
}

# Output the chained network's slug for downstream use
output "subnet_slug" {
  value = zcp_network.subnet.id
}

output "vpc_slug" {
  value = zcp_vpc.v.id
}
