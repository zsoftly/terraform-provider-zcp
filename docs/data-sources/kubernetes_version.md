---
page_title: "zcp_kubernetes_version Data Source"
description: |-
  Look up an available ZCP Kubernetes version.
---

# zcp_kubernetes_version (Data Source)

Looks up an available Kubernetes version by slug or version string, for use with
`zcp_kubernetes_cluster`.

## Example Usage

```terraform
data "zcp_kubernetes_version" "latest" {
  version = "1.32.0"
}

resource "zcp_kubernetes_cluster" "main" {
  # ...
  version = data.zcp_kubernetes_version.latest.slug
}
```

## Schema

### Optional

- `slug` (String) Version slug. Exactly one of `slug` or `version` must be set.
- `version` (String) Version string (e.g. `1.32.0`). Exactly one of `slug` or
  `version` must be set.

### Read-Only

- `id` (String) Version ID.
- `name` (String) Version display name.
- `cluster_version_id` (String) Kubernetes cluster version ID.
- `region_id` (String) Region ID the version is available in.
