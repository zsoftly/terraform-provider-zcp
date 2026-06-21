terraform {
  required_providers {
    zcp = {
      source = "zsoftly/zcp"
    }
  }
}

provider "zcp" {
  api_url         = "https://api.zcp.zsoftly.ca/api"
  bearer_token    = var.zcp_token
  default_project = "default-9"
}

variable "zcp_token" {
  type      = string
  sensitive = true
}

variable "ssh_public_key" {
  type = string
}
