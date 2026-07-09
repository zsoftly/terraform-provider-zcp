---
page_title: "zcp_iso Resource"
description: |-
  Register and manage ISO images on ZCP.
---

# zcp_iso

Registers an ISO image from a URL. The permission flags (`password_enabled`,
`is_extractable`, `is_bootable`) update in place. Every other change forces
replacement.

## Example Usage

```terraform
resource "zcp_iso" "rescue" {
  name           = "rescue-iso"
  url            = "https://mirror.example.com/rescue.iso"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  billing_cycle  = "hourly"
}
```

## Import

Import using `<slug>/<region>/<cloud_provider>/<billing_cycle>[/<project>]`:

```shell
terraform import zcp_iso.rescue rescue-iso-i1/yow-1/zsoftly/hourly
```

The API never returns `os_type_id`, `operating_system`, or
`operating_system_version`, so the first apply after import records their
configured values in state without replacing the ISO. Later changes to them
force replacement as usual.

## Schema

### Required

- `name` (String) ISO display name. Changing this forces replacement.
- `url` (String) HTTP(S) URL the platform downloads the ISO from. Changing this
  forces replacement.
- `cloud_provider` (String) Cloud provider slug. Changing this forces
  replacement.
- `region` (String) Region slug (e.g. `yow-1`). Changing this forces
  replacement.
- `billing_cycle` (String) Billing cycle (e.g. `hourly`, `monthly`). Changing
  this forces replacement.

### Optional

- `description` (String) Human-readable description. Changing this forces
  replacement.
- `project` (String) Project slug. Inherits from the provider `default_project`
  if omitted. Changing this forces replacement.
- `os_type_id` (String) Operating system type ID. Changing this forces
  replacement.
- `operating_system` (String) Operating system name (e.g. `Ubuntu`). Changing
  this forces replacement.
- `operating_system_version` (String) Operating system version (e.g. `24.04`).
  Changing this forces replacement.
- `password_enabled` (Boolean) Whether password reset is supported. Updated in
  place.
- `is_extractable` (Boolean) Whether the ISO is downloadable by other users.
  Updated in place.
- `is_bootable` (Boolean) Whether the ISO is bootable. Defaults to `true`.
  Updated in place.
- `timeouts` (Block) Configurable `create`, `update`, and `delete` timeouts.

### Read-Only

- `id` (String) ISO slug.
- `state` (String) Current state of the ISO.
