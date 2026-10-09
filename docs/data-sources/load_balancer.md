---
page_title: "zcp_load_balancer Data Source"
description: |-
  Look up an existing ZCP load balancer by slug.
---

# zcp_load_balancer (Data Source)

Looks up one load balancer by slug. The data source returns its nested rules,
including each rule ID for use with `zcp_load_balancer_attachment`.

## Example Usage

```terraform
data "zcp_load_balancer" "existing" {
  slug = var.load_balancer_slug
}

resource "zcp_load_balancer_attachment" "web" {
  load_balancer   = data.zcp_load_balancer.existing.id
  rule            = data.zcp_load_balancer.existing.rules[0].id
  virtual_machine = zcp_instance.web.id
  cloud_provider  = zcp_instance.web.cloud_provider
  region          = zcp_instance.web.region
}
```

## Schema

### Required

- `slug` (String) Load balancer slug.

### Read-Only

- `id` (String) Load balancer slug (same as `slug`).
- `name` (String) Load balancer display name.
- `state` (String) Current load balancer state.
- `public_ip` (String) Bound public IP address, when present.
- `region` (String) Region slug, when returned by the API.
- `project` (String) Project slug, when returned by the API.
- `cloud_provider` (String) Cloud provider slug, when returned by the API.
- `rules` (List of Object) Rules returned with the load balancer. Each object
  contains `id`, `name`, `public_port`, `private_port`, `protocol`, `algorithm`,
  `sticky_method`, `enable_tls`, and `enable_proxy`.
