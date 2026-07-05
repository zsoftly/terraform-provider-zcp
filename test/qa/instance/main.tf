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

resource "zcp_ssh_key" "k" {
  name       = "qa-inst-key"
  public_key = file(pathexpand("~/.ssh/id_ed25519.pub"))
  region     = data.zcp_region.yow.slug
}

resource "zcp_network" "net" {
  name           = "qa-inst-net"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  network_plan   = "inet-yow"
  billing_cycle  = "hourly"
}

# Attaches to the managed network above (leak-free). assign_public_ip toggles the
# public IP (defaults to true).
resource "zcp_instance" "vm" {
  name             = "qa-instance"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  template         = "ubuntu-2404-lts"
  plan             = "ci1xs"
  billing_cycle    = "hourly"
  network          = zcp_network.net.id
  storage_category = "nvme"
  ssh_key          = zcp_ssh_key.k.name
  assign_public_ip = true

  timeouts {
    create = "30m"
  }
}

output "id" { value = zcp_instance.vm.id }
output "state" { value = zcp_instance.vm.state }
output "private_ip" { value = zcp_instance.vm.private_ip }
output "public_ip" { value = zcp_instance.vm.public_ip }
