# DNS domains live in the account-level `default` region.
data "zcp_region" "default" {
  slug = "default"
}

resource "zcp_dns_domain" "example" {
  name           = "example.com"
  cloud_provider = data.zcp_region.default.cloud_provider
  region         = data.zcp_region.default.slug
}
