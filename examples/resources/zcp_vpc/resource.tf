data "zcp_region" "yow" {
  slug = "yow-1"
}

data "zcp_project" "default" {
  slug = "default"
}

# ── Example 1: minimal VPC ────────────────────────────────────────────────────
# cidr is the base network address (no prefix notation); size is the prefix length.
# Run `zcp plan router` to list available VPC plans.
# Available plans: virtual-private-cloud-vpc-1 (5 Gbps), virtual-private-cloud-vpc (50 Mbps)
resource "zcp_vpc" "main" {
  name             = "main-vpc"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  cidr             = "10.1.0.1"
  size             = "24"
  type             = "Vpc"
  billing_cycle    = "hourly"
  storage_category = "nvme"
  plan             = "virtual-private-cloud-vpc-1"
}

# ── Example 2: VPC with description ──────────────────────────────────────────
resource "zcp_vpc" "prod" {
  name             = "prod-vpc"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  cidr             = "10.10.0.1"
  size             = "24"
  type             = "Vpc"
  billing_cycle    = "monthly"
  storage_category = "nvme"
  plan             = "virtual-private-cloud-vpc-1"
  description      = "Production VPC"
}

# ── Example 3: VPC scoped to an explicit project ──────────────────────────────
resource "zcp_vpc" "staging" {
  name             = "staging-vpc"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  cidr             = "10.20.0.1"
  size             = "24"
  type             = "Vpc"
  billing_cycle    = "hourly"
  storage_category = "nvme"
  plan             = "virtual-private-cloud-vpc-1"
  project          = data.zcp_project.default.slug
  description      = "Staging VPC"
}

# ── Example 4: VPCs per environment using for_each ────────────────────────────
locals {
  envs = {
    dev  = { cidr = "10.100.0.1", description = "Development VPC" }
    stg  = { cidr = "10.101.0.1", description = "Staging VPC" }
    prod = { cidr = "10.102.0.1", description = "Production VPC" }
  }
}

resource "zcp_vpc" "env" {
  for_each         = local.envs
  name             = "${each.key}-vpc"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  cidr             = each.value.cidr
  size             = "24"
  type             = "Vpc"
  billing_cycle    = "hourly"
  storage_category = "nvme"
  plan             = "virtual-private-cloud-vpc-1"
  description      = each.value.description
}

# ── Outputs ───────────────────────────────────────────────────────────────────
output "main_vpc_id" {
  description = "Slug of the main VPC."
  value       = zcp_vpc.main.id
}

output "main_vpc_status" {
  value = zcp_vpc.main.status
}

output "env_vpc_ids" {
  value = { for k, v in zcp_vpc.env : k => v.id }
}
