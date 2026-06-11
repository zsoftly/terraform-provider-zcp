# cloud_provider comes from the region — no hardcoding required.
data "zcp_region" "yow" {
  slug = "yow-1"
}

data "zcp_project" "default" {
  slug = "default"
}

# ── Example 1: minimal isolated network ───────────────────────────────────────
resource "zcp_network" "app" {
  name           = "app-network"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
}

# ── Example 2: network with a specific category and description ───────────────
resource "zcp_network" "db" {
  name           = "db-network"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  category_slug  = "isolated"
  description    = "Private network for database tier"
}

# ── Example 3: scoped to an explicit project ──────────────────────────────────
resource "zcp_network" "staging" {
  name           = "staging-network"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  project        = data.zcp_project.default.slug
  description    = "Staging environment network"
}

# ── Example 4: multiple networks across tiers ─────────────────────────────────
locals {
  tiers = {
    web = "Web tier — public-facing services"
    app = "App tier — internal API servers"
    db  = "DB tier  — database nodes"
  }
}

resource "zcp_network" "tier" {
  for_each       = local.tiers
  name           = "${each.key}-network"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  description    = each.value
}

# ── Outputs ───────────────────────────────────────────────────────────────────
output "app_network_cidr" {
  value = zcp_network.app.cidr
}

output "tier_network_ids" {
  value = { for k, v in zcp_network.tier : k => v.id }
}
