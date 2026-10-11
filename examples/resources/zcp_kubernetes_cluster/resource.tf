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
# Create blocks until the cluster reaches the Running state. The separate fixed
# plans and root volume define the cluster shape and force replacement if changed.
resource "zcp_kubernetes_cluster" "main" {
  name               = "main-cluster"
  cloud_provider     = data.zcp_region.yow.cloud_provider
  region             = data.zcp_region.yow.slug
  version            = "v1.36.4"
  control_plane_plan = "k8s-cpi-yow-1"
  worker_plan        = "k8s-li-yow-1"
  storage_plan       = "b1g1"
  root_disk_size     = 100
  billing_cycle      = "hourly"
  workers            = 3
  storage_category   = "nvme"
  ssh_key            = zcp_ssh_key.k8s.name
}

# ── High-availability cluster ─────────────────────────────────────────────────
resource "zcp_kubernetes_cluster" "ha" {
  name               = "ha-cluster"
  cloud_provider     = data.zcp_region.yow.cloud_provider
  region             = data.zcp_region.yow.slug
  version            = "v1.36.4"
  control_plane_plan = "k8s-cpi-yow-1"
  worker_plan        = "k8s-li-yow-1"
  storage_plan       = "b1g1"
  root_disk_size     = 100
  billing_cycle      = "hourly"
  workers            = 3
  control_nodes      = 3
  ha                 = true
  storage_category   = "nvme"
  ssh_key            = zcp_ssh_key.k8s.name
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
#   '<slug>/<cloud_provider>/<region>/<version>/<control_plane_plan>/<worker_plan>/<storage_plan>/<root_disk_size>/<billing_cycle>/<storage_category>/<project>/<ssh_key>/<enable_csi>/<ha>'
#   e.g. terraform import zcp_kubernetes_cluster.main \
#     'main-cluster-abc/nimbo/yow-1/v1.36.4/k8s-cpi-yow-1/k8s-li-yow-1/b1g1/100/hourly/nvme/default-9/k8s-key/false/false'
