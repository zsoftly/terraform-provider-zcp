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

# Negative test: a non-existent template slug must surface the API error
# message (not a provider panic) and create nothing.
resource "zcp_instance" "bad" {
  name             = "qa-bad-template"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  template         = "ubuntu-does-not-exist-9999"
  plan             = "ci1xs"
  billing_cycle    = "hourly"
  network_plan     = "pnet-yow"
  storage_category = "nvme"

  timeouts {
    create = "5m"
  }
}
