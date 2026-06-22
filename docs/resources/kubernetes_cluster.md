---
page_title: "zcp_kubernetes_cluster Resource"
description: |-
  Create and manage ZCP managed Kubernetes clusters.
---

# zcp_kubernetes_cluster

Manages a ZCP managed Kubernetes cluster. Create **blocks until the cluster reaches the `Running` state**, at which point `api_endpoint` and `ip_address` are populated in state.

Changing `workers` scales the cluster **in place** (an update, not a replacement) and blocks until scaling completes. Changing `version` or `region` forces a new cluster.

`name`, `cloud_provider`, `region`, `version`, `plan`, `billing_cycle`, `storage_category`, `project`, `ssh_key`, `control_nodes`, and `ha` are immutable. Changing any of them forces replacement.

~> **Note on write-only fields:** the create-only inputs above are sent to the API on creation but are preserved from state on refresh to avoid format-driven diffs.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_kubernetes_cluster" "main" {
  name             = "main-cluster"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  version          = "v1.36.1"
  plan             = "k8s-li-yow-1"
  billing_cycle    = "hourly"
  workers          = 3
  storage_category = "pro-nvme"
  ssh_key          = "k8s-key"
}
```

## Import

```shell
terraform import zcp_kubernetes_cluster.main '<slug>/<cloud_provider>/<region>/<version>/<plan>/<billing_cycle>/<storage_category>[/<project>/<ssh_key>]'
```

`<slug>/<cloud_provider>/<region>/<version>/<plan>/<billing_cycle>/<storage_category>` are required. `workers`, `control_nodes`, `ha`, `name`, `state` and the endpoints come from the subsequent read.

## Schema

### Required

- `name` (String) Display name for the cluster. Changing this forces replacement.
- `cloud_provider` (String) Cloud provider slug. Use `data.zcp_region.<name>.cloud_provider`. Changing this forces replacement.
- `region` (String) Region slug (e.g. `yow-1`). Changing this forces replacement.
- `version` (String) Kubernetes version (e.g. `v1.36.1`). Changing this forces replacement.
- `plan` (String) Cluster node plan slug (e.g. `k8s-li-yow-1`). Changing this forces replacement.
- `billing_cycle` (String) Billing cycle (`hourly` or `monthly`). Changing this forces replacement.
- `workers` (Number) Number of worker nodes (>= 1). Changing this scales the cluster in place.
- `storage_category` (String) Storage category slug (e.g. `pro-nvme`, `nvme`, `ssd`). Changing this forces replacement.

### Optional

- `project` (String) Project slug. Inherits from the provider `default_project` if omitted. Changing this forces replacement.
- `ssh_key` (String) Name of an existing SSH key for node login (see `zcp_ssh_key`). Changing this forces replacement.
- `control_nodes` (Number) Number of control-plane nodes (default 1, use >= 3 for HA). Changing this forces replacement.
- `ha` (Boolean) Enable high availability. Changing this forces replacement.
- `timeouts` (Block) Configurable `create`, `update`, and `delete` timeouts. Create defaults to 20m.

### Read-Only

- `id` (String) Cluster slug (unique identifier).
- `slug` (String) Cluster slug (same value as `id`).
- `state` (String) Current cluster state (e.g. `Running`).
- `api_endpoint` (String) Kubernetes API server endpoint, populated once Running.
- `ip_address` (String) Public IP address of the cluster, populated once Running.
