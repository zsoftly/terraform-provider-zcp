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
  default_project = var.default_project
}

variable "zcp_token" {
  type      = string
  sensitive = true
}

variable "default_project" {
  type    = string
  default = "default-9"
}

variable "ssh_public_key" {
  type = string
}
