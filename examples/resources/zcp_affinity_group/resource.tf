data "zcp_region" "yow" {
  slug = "yow-1"
}

# Spread web instances across hypervisor hosts.
resource "zcp_affinity_group" "web_spread" {
  name           = "web-anti-affinity"
  type           = "host anti-affinity"
  description    = "Spread web instances across hosts"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
}
