---
page_title: "zcp_load_balancer_attachment Resource"
description: |-
  Attach ZCP instances to load balancer rules.
---

# zcp_load_balancer_attachment

Attaches a single instance to a load balancer rule. Use one attachment per
instance/rule pair, with `for_each` for fleets.

~> The API does not expose rule membership, so attachments changed outside
Terraform are not detected as drift. The resource is removed from state when its
rule or load balancer disappears.

## Example Usage

```terraform
resource "zcp_load_balancer_attachment" "web" {
  for_each = zcp_instance.web

  load_balancer   = zcp_load_balancer.web.id
  rule            = zcp_load_balancer.web.rule_id
  virtual_machine = each.value.id
  cloud_provider  = data.zcp_region.yow.cloud_provider
  region          = data.zcp_region.yow.slug
}
```

## Import

Import using
`<load-balancer-slug>/<rule-id>/<vm-slug>/<cloud_provider>/<region>[/<project>]`:

```shell
terraform import zcp_load_balancer_attachment.web web-lb-a1b2/<rule-id>/vm1-abc/zsoftly/yow-1
```

## Schema

### Required

- `load_balancer` (String) Parent load balancer slug. Changing this forces
  replacement.
- `rule` (String) Rule ID to attach to (e.g. `zcp_load_balancer.web.rule_id`).
  Changing this forces replacement.
- `virtual_machine` (String) Instance slug to attach. Changing this forces
  replacement.
- `cloud_provider` (String) Cloud provider slug. Changing this forces
  replacement.
- `region` (String) Region slug. Changing this forces replacement.

### Optional

- `project` (String) Project slug. Inherits from the provider `default_project`
  if omitted. Changing this forces replacement.
- `timeouts` (Block) Configurable `create` and `delete` timeouts.

### Read-Only

- `id` (String) Synthetic attachment ID
  (`<load_balancer>/<rule>/<virtual_machine>`).
