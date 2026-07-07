---
page_title: "zcp_affinity_group Resource"
description: |-
  Create and manage ZCP affinity groups for instance placement.
---

# zcp_affinity_group

Manages a ZCP affinity group. Affinity groups influence host placement for
instances, for example spreading web servers across hypervisors with
`host anti-affinity`. The API has no update endpoint, so every change forces
replacement.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_affinity_group" "web_spread" {
  name           = "web-anti-affinity"
  type           = "host anti-affinity"
  description    = "Spread web instances across hosts"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
}
```

## Import

Import using `<slug>/<region>/<cloud_provider>[/<project>]`. Omit `<project>`
when the config relies on the provider `default_project`:

```shell
terraform import zcp_affinity_group.web_spread web-anti-affinity-x1y2/yow-1/zsoftly
```

## Schema

### Required

- `name` (String) Display name. Changing this forces replacement.
- `type` (String) Affinity group type (e.g. `host anti-affinity`). Changing this
  forces replacement.
- `cloud_provider` (String) Cloud provider slug. Changing this forces
  replacement.
- `region` (String) Region slug (e.g. `yow-1`). Changing this forces
  replacement.

### Optional

- `description` (String) Human-readable description. Changing this forces
  replacement.
- `project` (String) Project slug. Inherits from the provider `default_project`
  if omitted. Changing this forces replacement.
- `timeouts` (Block) Configurable `create` and `delete` timeouts.

### Read-Only

- `id` (String) Affinity group slug.
- `state` (String) Current state of the affinity group.
