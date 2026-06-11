terraform {
  required_providers {
    zcp = {
      source = "registry.opentofu.org/zsoftly/zcp"
    }
  }
}

provider "zcp" {
  api_url        = "https://api.zcp.zsoftly.ca/api"
  bearer_token   = var.zcp_token
  default_project = "default-2"
}

variable "zcp_token" {
  type      = string
  sensitive = true
}
