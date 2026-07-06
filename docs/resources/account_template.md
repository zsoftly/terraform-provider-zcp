---
page_title: "zcp_account_template Resource"
description: |-
  Register custom templates in the ZCP account catalogue.
---

# zcp_account_template

Registers a custom template in the account catalogue, either from an image URL or captured from an existing instance. Templates are immutable, so every change forces replacement.

## Example Usage

```terraform
# Capture a golden image from a prepared instance.
resource "zcp_account_template" "golden_web" {
  name            = "golden-web"
  virtual_machine = zcp_instance.web.id
  cloud_provider  = data.zcp_region.yow.cloud_provider
  region          = data.zcp_region.yow.slug
  billing_cycle   = "monthly"
}
```

## Import

Import using `<slug>/<region>/<cloud_provider>/<billing_cycle>[/<project>]`:

```shell
terraform import zcp_account_template.golden_web golden-web-t1/yow-1/zsoftly/monthly
```

## Schema

### Required

- `name` (String) Template display name. Changing this forces replacement.
- `cloud_provider` (String) Cloud provider slug. Changing this forces replacement.
- `region` (String) Region slug (e.g. `yow-1`). Changing this forces replacement.
- `billing_cycle` (String) Billing cycle (e.g. `hourly`, `monthly`). Changing this forces replacement.

### Optional

- `url` (String) HTTP(S) URL of the source image. Exactly one of `url` or `virtual_machine` must be set. Changing this forces replacement.
- `virtual_machine` (String) Slug of an instance to capture the template from. Exactly one of `url` or `virtual_machine` must be set. Changing this forces replacement.
- `description` (String) Human-readable description. Changing this forces replacement.
- `project` (String) Project slug. Inherits from the provider `default_project` if omitted. Changing this forces replacement.
- `os_type_id` (String) Operating system type ID. Changing this forces replacement.
- `operating_system` (String) Operating system name. Changing this forces replacement.
- `operating_system_version` (String) Operating system version. Changing this forces replacement.
- `format` (String) Image format (e.g. `QCOW2`). Changing this forces replacement.
- `password_enabled` (Boolean) Whether password reset is supported. Changing this forces replacement.

### Read-Only

- `id` (String) Account template slug.
- `state` (String) Current state of the template.
