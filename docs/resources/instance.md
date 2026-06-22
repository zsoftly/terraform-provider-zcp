---
page_title: "zcp_instance Resource"
description: |-
  Create and manage ZCP virtual machine instances.
---

# zcp_instance

Manages a ZCP virtual machine instance. Create **blocks until the instance reaches the `Running` state** (mirroring the CLI's `--wait`). `private_ip` and `public_ip` are populated in state once the resource finishes applying.

The resource maps the CLI's instance operations to Terraform's declarative model:

- **In-place updates:** `name` (display name), `plan` + `billing_cycle` (resize), `user_data` (change-startup-script), and `tags` (tag-create / tag-delete).
- **Force replacement:** `cloud_provider`, `region`, `template` (an OS/template change reprovisions the disk), and the other create-only inputs (`project`, `ssh_key`, `network_plan`, `storage_category`).

**Power state is not managed by Terraform.** The provider reports the instance's runtime state (running/stopped) read-only in `state`. Running `apply` against a stopped or running instance produces no diff and never starts or stops it. The provider only stops and restarts the VM **internally during a resize**. Changing `plan` (and/or `billing_cycle`) stops the instance, changes its compute offering, and restarts it to its previous state, like changing an `aws_instance` `instance_type`. `tags` are optional.

~> **Hostname is set once at creation** (from `name`) and is not changed afterward. Only the display `name` is mutable. This matches CloudStack/EC2 behaviour.

Imperative, non-declarative CLI operations (`reboot`/`reset`, `change-password`, `ssh`, `logs`, and `addons`) are not modeled as resource attributes. Use the `zcp` CLI for those.

~> **Note on write-only fields:** the create-only inputs are sent to the API on creation but are not all echoed back in a comparable form on refresh. They are preserved in Terraform state.

~> **Public-platform requirement:** although `network_plan` and `storage_category` are schema-optional (private-cloud configurations may omit them), the public ZCP API **requires both** when creating an instance and returns a `422` validation error otherwise.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_instance" "web" {
  name             = "web-01"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  template         = "ubuntu-2404-lts"
  plan             = "ca1hxs"
  billing_cycle    = "hourly"
  network_plan     = "pnet-yow"
  storage_category = "nvme"
  ssh_key          = "deploy-key"
}
```

## Import

Create-only attributes the API does not return are supplied positionally in the import ID (slash-separated, trailing/empty segments allowed):

```shell
terraform import zcp_instance.web '<slug>/<cloud_provider>/<region>/<template>[/<plan>/<billing_cycle>/<project>/<ssh_key>/<network_plan>/<storage_category>]'
```

`<slug>/<cloud_provider>/<region>/<template>` are required. `name`, `state` and the IPs come from the subsequent read.

## Schema

### Required

- `name` (String) Display name of the instance. Updated in place. The hostname is set from this value at creation and is not changed afterward.
- `cloud_provider` (String) Cloud provider slug. Use `data.zcp_region.<name>.cloud_provider`. Changing this forces replacement.
- `region` (String) Region slug (e.g. `yow-1`). Changing this forces replacement.
- `template` (String) Template (OS image) slug. See `data.zcp_template`. Changing this forces replacement.
- `plan` (String) Compute plan slug. Run `zcp plan vm` to list values. Updated in place (resize): the provider stops the instance, changes the offering, and restarts it to its prior running/stopped state.
- `billing_cycle` (String) Billing cycle (`hourly` or `monthly`). Updated in place together with `plan`.

### Optional

- `project` (String) Project slug. Inherits from the provider `default_project` if omitted. Changing this forces replacement.
- `ssh_key` (String) Name of an existing SSH key to attach for login (see `zcp_ssh_key`). Changing this forces replacement.
- `network_plan` (String) Network plan slug (e.g. `pnet-yow`). Required by the public API. Run `zcp plan network` to list values. Changing this forces replacement.
- `storage_category` (String) Storage category slug (e.g. `nvme`, `pro-nvme`). Required by the public API. Changing this forces replacement.
- `user_data` (String) Startup script content (cloud-init / bash). Updated in place via change-startup-script (takes effect on next boot).
- `tags` (Map of String) Key/value tags applied via tag-create / tag-delete. **Write-only:** the API does not return tags on read, so they are tracked in state but not refreshed (no drift detection) and are not populated on import.
- `timeouts` (Block) Configurable `create`, `update`, and `delete` timeouts. Create defaults to 20m.

### Read-Only

- `id` (String) Instance slug (unique identifier).
- `slug` (String) Instance slug (same value as `id`).
- `state` (String) Current runtime state of the instance (e.g. `Running`, `Stopped`), reported for information only. Terraform does not reconcile or manage power state.
- `private_ip` (String) Private IP of the instance's default network.
- `public_ip` (String) Public IP of the instance, if assigned.
