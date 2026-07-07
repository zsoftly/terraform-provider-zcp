---
page_title: "zcp_load_balancer Resource"
description: |-
  Create and manage ZCP load balancers.
---

# zcp_load_balancer

Manages a ZCP load balancer together with its initial rule (the API requires at
least one rule at create time). Add further rules with `zcp_load_balancer_rule`
and attach instances with `zcp_load_balancer_attachment`. The API has no update
endpoint for load balancers, so every change forces replacement.

By default the load balancer acquires a fresh public IP. Set `ip_address` to
bind an existing public IP instead.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_load_balancer" "web" {
  name           = "web-lb"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  network        = zcp_network.prod.id
  billing_cycle  = "hourly"

  rule_name    = "https"
  public_port  = "443"
  private_port = "8443"
  algorithm    = "roundrobin"
}

resource "zcp_load_balancer_attachment" "web_vm1" {
  load_balancer   = zcp_load_balancer.web.id
  rule            = zcp_load_balancer.web.rule_id
  virtual_machine = zcp_instance.web.id
  cloud_provider  = data.zcp_region.yow.cloud_provider
  region          = data.zcp_region.yow.slug
}
```

## Import

Import using the composite ID below. The optional tail fields seed write-only
attributes for a zero-diff plan:

```shell
terraform import zcp_load_balancer.web \
  web-lb-a1b2/zsoftly/yow-1/prod-net/hourly/https/443/8443/roundrobin
```

Format:
`<slug>/<cloud_provider>/<region>/<network>/<billing_cycle>/<rule_name>/<public_port>/<private_port>/<algorithm>[/<plan>/<project>]`

## Schema

### Required

- `name` (String) Display name. Changing this forces replacement.
- `cloud_provider` (String) Cloud provider slug. Changing this forces
  replacement.
- `region` (String) Region slug (e.g. `yow-1`). Changing this forces
  replacement.
- `network` (String) Slug of the network the balanced instances live in.
  Changing this forces replacement.
- `billing_cycle` (String) Billing cycle (e.g. `hourly`, `monthly`). Changing
  this forces replacement.
- `rule_name` (String) Name of the initial rule. Changing this forces
  replacement.
- `public_port` (String) Public port of the initial rule. Changing this forces
  replacement.
- `private_port` (String) Private port of the initial rule. Changing this forces
  replacement.
- `algorithm` (String) Balancing algorithm: `roundrobin`, `leastconn`, or
  `source`. Changing this forces replacement.

### Optional

- `project` (String) Project slug. Inherits from the provider `default_project`
  if omitted. Changing this forces replacement.
- `plan` (String) Load balancer plan slug. Changing this forces replacement.
- `acquire_new_ip` (Boolean) Acquire a new public IP. Defaults to `true` when
  `ip_address` is not set. Conflicts with `ip_address`. Changing this forces
  replacement.
- `ip_address` (String) Existing public IP slug to bind instead of acquiring a
  new one. Changing this forces replacement.
- `protocol` (String) Protocol of the initial rule (e.g. `tcp`). Changing this
  forces replacement.
- `sticky_method` (String) Session stickiness method (e.g. `LbCookie`,
  `SourceBased`). Changing this forces replacement.
- `enable_tls` (Boolean) Enable TLS on the initial rule. Changing this forces
  replacement.
- `enable_proxy` (Boolean) Enable the PROXY protocol on the initial rule.
  Changing this forces replacement.
- `virtual_machines` (List of String) Instance slugs to attach to the initial
  rule at create time. Manage attachments after create with
  `zcp_load_balancer_attachment`. Changing this forces replacement.
- `timeouts` (Block) Configurable `create` and `delete` timeouts.

### Read-Only

- `id` (String) Load balancer slug.
- `state` (String) Current state.
- `public_ip` (String) Public IP address bound to the load balancer.
- `rule_id` (String) ID of the initial rule, usable with
  `zcp_load_balancer_attachment`.
