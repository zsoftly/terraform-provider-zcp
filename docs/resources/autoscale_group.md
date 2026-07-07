---
page_title: "zcp_autoscale_group Resource"
description: |-
  Create and manage ZCP autoscale groups.
---

# zcp_autoscale_group

Manages a ZCP autoscale group. `plan`, `template`, and `enabled` update in
place; other changes force replacement. Add scaling rules with
`zcp_autoscale_policy` (scale up) and `zcp_autoscale_condition` (scale down).

## Example Usage

```terraform
resource "zcp_autoscale_group" "web" {
  name           = "web-asg"
  plan           = "ci1xs"
  template       = "ubuntu-24"
  min_instances  = 1
  max_instances  = 5
  zone           = "yow-zone-1"
  network        = zcp_network.prod.id
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
}
```

## Import

Import using `<slug>/<region>/<cloud_provider>[/<project>]`:

```shell
terraform import zcp_autoscale_group.web web-asg-a1/yow-1/zsoftly
```

## Schema

### Required

- `name` (String) Display name. Changing this forces replacement.
- `plan` (String) Compute plan slug for scaled instances. Updated in place.
- `template` (String) Template slug for scaled instances. Updated in place.
- `min_instances` (Number) Minimum instance count. Changing this forces
  replacement.
- `max_instances` (Number) Maximum instance count. Changing this forces
  replacement.
- `zone` (String) Zone slug the group scales in. Changing this forces
  replacement.
- `cloud_provider` (String) Cloud provider slug. Changing this forces
  replacement.
- `region` (String) Region slug (e.g. `yow-1`). Changing this forces
  replacement.

### Optional

- `cooldown_period` (Number) Cooldown in seconds between scaling actions.
  Changing this forces replacement.
- `network` (String) Network slug scaled instances join. Changing this forces
  replacement.
- `enabled` (Boolean) Whether autoscaling is active. Toggled in place.
- `project` (String) Project slug. Inherits from the provider `default_project`
  if omitted. Changing this forces replacement.
- `timeouts` (Block) Configurable `create`, `update`, and `delete` timeouts.

### Read-Only

- `id` (String) Autoscale group slug.
- `state` (String) Current state of the group.
- `current_count` (Number) Current number of instances in the group.
