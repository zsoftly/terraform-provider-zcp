terraform {
  required_providers {
    zcp = { source = "registry.terraform.io/zsoftly/zcp" }
  }
}

variable "zcp_token" {
  type      = string
  sensitive = true
}

# Scale by overriding this: terraform apply -var 'workers=2'
variable "workers" {
  type    = number
  default = 1
}

provider "zcp" {
  api_url         = "https://api.zcp.zsoftly.ca/api"
  bearer_token    = var.zcp_token
  default_project = "default-9"
}

data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_ssh_key" "k" {
  name       = "qa-k8s-key"
  public_key = file(pathexpand("~/.ssh/id_ed25519.pub"))
  region     = data.zcp_region.yow.slug
}

resource "zcp_kubernetes_cluster" "c" {
  name             = "qa-k8s"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  version          = "v1.36.1"
  plan             = "k8s-li-yow-1"
  billing_cycle    = "hourly"
  workers          = var.workers
  storage_category = "nvme"
  ssh_key          = zcp_ssh_key.k.name

  timeouts {
    create = "30m"
    update = "30m"
    delete = "30m"
  }
}

output "state" { value = zcp_kubernetes_cluster.c.state }
output "workers" { value = zcp_kubernetes_cluster.c.workers }
output "api_endpoint" { value = zcp_kubernetes_cluster.c.api_endpoint }
output "ip_address" { value = zcp_kubernetes_cluster.c.ip_address }
