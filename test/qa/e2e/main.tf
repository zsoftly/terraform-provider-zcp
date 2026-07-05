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

# 1. SSH key
resource "zcp_ssh_key" "k" {
  name       = "qa-e2e-key"
  public_key = file(pathexpand("~/.ssh/id_ed25519.pub"))
  region     = data.zcp_region.yow.slug
}

# 2. Network (standalone isolated network)
resource "zcp_network" "net" {
  name           = "qa-e2e-net"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  network_plan   = "inet-yow"
  billing_cycle  = "hourly"
}

# 3. Instance — attaches to the network above (no auto-created network, so destroy
#    leaves no orphan). depends on the SSH key + network.
resource "zcp_instance" "vm" {
  name             = "qa-e2e-instance"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  template         = "ubuntu-2404-lts"
  plan             = "ci1xs"
  billing_cycle    = "hourly"
  network          = zcp_network.net.id
  storage_category = "nvme"
  ssh_key          = zcp_ssh_key.k.name

  timeouts {
    create = "30m"
  }
}

# 4. Volume attached to the instance (depends on the instance)
resource "zcp_volume" "vol" {
  name             = "qa-e2e-volume"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  billing_cycle    = "hourly"
  storage_category = "nvme"
  size             = 10
  vm               = zcp_instance.vm.id
}

# 5. Kubernetes cluster (depends on the SSH key)
resource "zcp_kubernetes_cluster" "c" {
  name             = "qa-e2e-k8s"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  version          = "v1.36.1"
  plan             = "k8s-li-yow-1"
  billing_cycle    = "hourly"
  workers          = 1
  storage_category = "nvme"
  ssh_key          = zcp_ssh_key.k.name

  timeouts {
    create = "30m"
    update = "30m"
    delete = "30m"
  }
}

output "instance_state" { value = zcp_instance.vm.state }
output "instance_private_ip" { value = zcp_instance.vm.private_ip }
output "instance_public_ip" { value = zcp_instance.vm.public_ip }
output "volume_id" { value = zcp_volume.vol.id }
output "network_id" { value = zcp_network.net.id }
output "k8s_state" { value = zcp_kubernetes_cluster.c.state }
output "k8s_endpoint" { value = zcp_kubernetes_cluster.c.api_endpoint }
