# cloud_provider is read from the region data source — no hardcoding.
data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_ssh_key" "k8s" {
  name       = "k8s-key"
  public_key = file(pathexpand("~/.ssh/id_ed25519.pub"))
  region     = data.zcp_region.yow.slug
}

# ── Managed Kubernetes cluster ────────────────────────────────────────────────
# Create blocks until the cluster reaches the Running state. Changing `workers`
# scales the cluster in place (it is NOT a replacement); changing `version` or
# `region` forces a new cluster.
resource "zcp_kubernetes_cluster" "main" {
  name             = "main-cluster"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  version          = "v1.36.1"
  plan             = "k8s-li-yow-1"
  billing_cycle    = "hourly"
  workers          = 3
  storage_category = "pro-nvme"
  ssh_key          = zcp_ssh_key.k8s.name
}

# ── High-availability cluster ─────────────────────────────────────────────────
resource "zcp_kubernetes_cluster" "ha" {
  name             = "ha-cluster"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  version          = "v1.36.1"
  plan             = "k8s-li-yow-1"
  billing_cycle    = "hourly"
  workers          = 3
  control_nodes    = 3
  ha               = true
  storage_category = "pro-nvme"
  ssh_key          = zcp_ssh_key.k8s.name
}

# ── Outputs ───────────────────────────────────────────────────────────────────
output "cluster_api_endpoint" {
  description = "Kubernetes API server endpoint."
  value       = zcp_kubernetes_cluster.main.api_endpoint
}

output "cluster_ip" {
  description = "Public IP address of the cluster."
  value       = zcp_kubernetes_cluster.main.ip_address
}

# ── Import ────────────────────────────────────────────────────────────────────
# terraform import zcp_kubernetes_cluster.main \
#   '<slug>/<cloud_provider>/<region>/<version>/<plan>/<billing_cycle>/<storage_category>[/<project>/<ssh_key>]'
#   e.g. terraform import zcp_kubernetes_cluster.main \
#     'main-cluster-abc/nimbo/yow-1/v1.36.1/k8s-li-yow-1/hourly/pro-nvme'
