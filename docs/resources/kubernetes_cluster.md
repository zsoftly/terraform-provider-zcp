---
page_title: "zcp_kubernetes_cluster Resource"
description: |-
  Create and manage ZCP managed Kubernetes clusters.
---

# zcp_kubernetes_cluster

Manages a ZCP managed Kubernetes cluster. Create **blocks until the cluster
reaches the `Running` state**, at which point `api_endpoint` and `ip_address`
are populated in state.

Changing `workers` scales a cluster with autoscaling disabled and waits for the
reported worker count. Changing `version` upgrades the cluster in place.
Disabling autoscaling first waits for the live worker count to settle, then
applies the configured fixed worker count. Changing a fixed node plan,
root-volume plan, root-disk size, or `region` forces replacement. The API has no
role-specific plan-update operation.

`name`, `cloud_provider`, `region`, `billing_cycle`, `storage_category`,
`project`, `ssh_key`, `control_nodes`, `ha`, and `enable_csi` are immutable.

The provider keeps the legacy `plan` field in state so it can read and destroy
clusters created by earlier releases. New clusters cannot use this field.

Destroy submits the platform's immediate Kubernetes service-cancellation request
and waits for the cluster to disappear. The provider does not store kubeconfig
or other cluster credentials.

Staging validation found that cluster cancellation left an empty network and its
active subscription. Separate network deletion was required to finish cleanup.
Check the remaining network and subscription after cluster destruction.

~> **Note on write-only fields:** the create-only inputs above are sent to the
API on creation but are preserved from state on refresh to avoid format-driven
diffs.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_kubernetes_cluster" "main" {
  name             = "main-cluster"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  version          = "v1.36.4"
  control_plane_plan = "k8s-cpi-yow-1"
  worker_plan        = "k8s-li-yow-1"
  storage_plan       = "b1g1"
  root_disk_size     = 100
  billing_cycle    = "hourly"
  workers          = 3
  storage_category = "nvme"
  ssh_key          = "k8s-key"
}
```

## Import

```shell
terraform import zcp_kubernetes_cluster.main '<slug>/<cloud_provider>/<region>/<version>/<plan>/<billing_cycle>/<storage_category>[/<project>/<ssh_key>]'
```

`<slug>/<cloud_provider>/<region>/<version>/<plan>/<billing_cycle>/<storage_category>`
are required. `workers`, `control_nodes`, `ha`, `name`, `state` and the
endpoints come from the subsequent read.

For a split-plan cluster, include every creation value the API does not return:

```shell
terraform import zcp_kubernetes_cluster.main '<slug>/<cloud_provider>/<region>/<version>/<control_plane_plan>/<worker_plan>/<storage_plan>/<root_disk_size>/<billing_cycle>/<storage_category>/<project>/<ssh_key>/<enable_csi>/<ha>'
```

## Schema

### Required

- `name` (String) Display name for the cluster. Changing this forces
  replacement.
- `cloud_provider` (String) Cloud provider slug. Use
  `data.zcp_region.<name>.cloud_provider`. Changing this forces replacement.
- `region` (String) Region slug (e.g. `yow-1`). Changing this forces
  replacement.
- `version` (String) Kubernetes version string (e.g. `v1.36.4`). Changing this
  upgrades the cluster in place.
- `control_plane_plan` (String) Fixed control-plane node plan slug. Changing
  this forces replacement.
- `worker_plan` (String) Fixed worker node plan slug. Changing this forces
  replacement.
- `storage_plan` (String) Fixed root-volume storage tier slug. `custom_plan` is
  not supported. Changing this forces replacement.
- `root_disk_size` (Number) Root-volume capacity in GB. Changing this forces
  replacement.
- `billing_cycle` (String) Billing cycle (`hourly` or `monthly`). Changing this
  forces replacement.
- `workers` (Number) Number of worker nodes (>= 1). Changing this scales the
  cluster in place.
- `storage_category` (String) Storage category slug. Region-specific: `nvme` in
  yow-1, `pro-nvme`/`premium-ssd` in yul-1. Changing this forces replacement.

### Optional

- `project` (String) Project slug. Inherits from the provider `default_project`
  if omitted. Changing this forces replacement.
- `ssh_key` (String) Name of an existing SSH key for node login (see
  `zcp_ssh_key`). Changing this forces replacement.
- `control_nodes` (Number) Number of control-plane nodes (default 1, use >= 2
  for HA). Changing this forces replacement.
- `ha` (Boolean) Enable high availability. Changing this forces replacement.
- `enable_csi` (Boolean) Enable CSI at creation. Changing this forces
  replacement.
- `autoscaling` (Boolean) Enable worker autoscaling. Enabling records the
  requested bounds and does not wait for worker scale-out.
- `min_workers` (Number) Minimum workers when autoscaling is enabled.
- `max_workers` (Number) Maximum workers when autoscaling is enabled.
- `timeouts` (Block) Configurable `create`, `update`, and `delete` timeouts.
  Create defaults to 20m.

### Read-Only

- `id` (String) Cluster slug (unique identifier).
- `slug` (String) Cluster slug (same value as `id`).
- `state` (String) Current cluster state (e.g. `Running`).
- `api_endpoint` (String) Kubernetes API server endpoint, populated once
  Running.
- `ip_address` (String) Public IP address of the cluster, populated once
  Running.
- `network_name` and `network_slug` identify the attached network when the API
  returns it.
- `root_volume_slugs` (List of String) Root volumes returned by the API.
