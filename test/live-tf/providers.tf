terraform {
  required_providers {
    zcp = {
      source = "registry.terraform.io/zsoftly/zcp"
    }
  }
}

provider "zcp" {
  api_url         = "https://api.zcp.zsoftly.ca/api"
  bearer_token    = var.zcp_token
  default_project = "default-2"
}

variable "zcp_token" {
  type      = string
  sensitive = true
}

# Provide via TF_VAR_vpn_password / TF_VAR_ipsec_psk (or a gitignored *.tfvars) —
# do not hardcode secrets in committed config.
variable "vpn_password" {
  type      = string
  sensitive = true
}

variable "ipsec_psk" {
  type      = string
  sensitive = true
}
