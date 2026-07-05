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

# Negative test: plan AND size together must fail at PLAN time (ValidateConfig),
# before any API call.
resource "zcp_volume" "conflict" {
  name             = "qa-volume-conflict"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  billing_cycle    = "hourly"
  storage_category = "nvme"
  plan             = "b1g1"
  size             = 10
}
