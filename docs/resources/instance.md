---
page_title: "zcp_instance Resource"
description: |-
  Create and manage ZCP virtual machine instances.
---

# zcp_instance

Manages a ZCP virtual machine instance. Create **blocks until the instance
reaches the `Running` state** (mirroring the CLI's `--wait`). `private_ip` and
`public_ip` are populated in state once the resource finishes applying.

The resource maps the CLI's instance operations to Terraform's declarative
model:

- **In-place updates:** `name` (display name), `plan` + `billing_cycle`
  (resize), `user_data` (change-startup-script), and `tags` (tag-create /
  tag-delete).
- **Force replacement:** `cloud_provider`, `region`, `template` (an OS/template
  change reprovisions the disk), and the other create-only inputs (`project`,
  `ssh_key`, `network`, `network_plan`, `network_type`, `vr_plan`, `networks`,
  `default_network`, `assign_public_ip`, `storage_category`).

**Networking.** `network_type` selects the network model. Leave it unset for
`Isolated`, or set `L2` for a plain network, or `Vpc` for a network behind a
virtual router. `network_type` is a plain optional attribute, not computed. An
omitted value is treated as `Isolated` but is not written to state, so upgrading
from a provider version that predates `network_type` never plans a replacement
for an existing instance. Attach the instance to networks you manage with
`network` (a single existing `zcp_network`) or `networks` (a list of them).
`networks` supersedes `network`, so set only one. `Isolated` and `L2` can
instead auto-create a network with `network_plan`. `Vpc` cannot use
`network_plan`. It auto-creates its virtual router with `vr_plan` instead. One
of `network_plan` (or `vr_plan` for `Vpc`), `network`, or `networks` is
required. An empty `networks = []` counts the same as omitting `networks`. With
more than one entry in `networks`, set `default_network` to one of them.
`default_network` must match `network` when `network` is set instead of
`networks`. Prefer `network` or `networks` over `network_plan`/`vr_plan`. An
auto-created network is not managed by Terraform, so destroy leaves it behind.
Set `assign_public_ip = false` for a private-only instance.
`network_type = "L2"` requires `assign_public_ip = false`. `assign_public_ip`
defaults to `true`, and the platform rejects a public IP on an `L2` network, so
leaving `assign_public_ip` unset also fails validation for `L2`. `network`,
`networks`, `network_plan`, and `vr_plan` may be unknown at `terraform validate`
or `terraform plan` time, for example `network = zcp_network.app.id` before that
resource exists. An unknown value there still satisfies the "one network source
is required" rule, since it may resolve to a value once applied.

**Destroy releases the auto-assigned public IP.** When `assign_public_ip` is
`true` (the default), the platform allocates a public IP at create time, and
destroy releases it through the service-cancellation workflow so it is not left
allocated and billed. A public IP attached via
`zcp_ip_address`/`zcp_ip_association` belongs to those resources and is
untouched. Set `assign_public_ip = false` to create the instance without an
auto-assigned public IP.

**Power state is not managed by Terraform.** The provider reports the instance's
runtime state (running/stopped) read-only in `state`. Running `apply` against a
stopped or running instance produces no diff and never starts or stops it. The
provider only stops and restarts the VM **internally during a resize**. Changing
`plan` (and/or `billing_cycle`) stops the instance, changes its compute
offering, and restarts it to its previous state, like changing an `aws_instance`
`instance_type`. `tags` are optional.

~> **Hostname is set once at creation** (from `name`) and is not changed
afterward. Only the display `name` is mutable. This matches how instance
hostnames behave on EC2 and most clouds.

Imperative, non-declarative CLI operations (`reboot`/`reset`, `change-password`,
`ssh`, `logs`, and `addons`) are not modeled as resource attributes. Use the
`zcp` CLI for those.

~> **Note on write-only fields:** the create-only inputs are sent to the API on
creation but are not all echoed back in a comparable form on refresh. They are
preserved in Terraform state.

~> **Public-platform requirement:** although the network attributes and
`storage_category` are schema-optional (private-cloud configurations may omit
them), the public ZCP API **requires** a network (`network` or `network_plan`)
and `storage_category` when creating an instance and returns a `422` validation
error otherwise.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_network" "app" {
  name           = "app-net"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  network_plan   = "inet-yow"
  billing_cycle  = "hourly"
}

resource "zcp_instance" "web" {
  name             = "web-01"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  template         = "ubuntu-2404-lts"
  plan             = "ci1xs"
  billing_cycle    = "hourly"
  network          = zcp_network.app.id
  storage_category = "nvme"
  ssh_key          = "deploy-key"
}
```

A `Vpc` instance uses `vr_plan` in place of `network_plan`:

```terraform
resource "zcp_instance" "vpc" {
  name             = "vpc-01"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  template         = "ubuntu-2404-lts"
  plan             = "ci1xs"
  billing_cycle    = "hourly"
  network_type     = "Vpc"
  vr_plan          = "vr-basic"
  storage_category = "nvme"
  ssh_key          = "deploy-key"
}
```

An instance can attach more than one network with `networks`. `default_network`
picks which one is the default:

```terraform
resource "zcp_network" "app_secondary" {
  name           = "app-net-2"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  network_plan   = "inet-yow"
  billing_cycle  = "hourly"
}

resource "zcp_instance" "multi_net" {
  name             = "multi-net-01"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  template         = "ubuntu-2404-lts"
  plan             = "ci1xs"
  billing_cycle    = "hourly"
  networks         = [zcp_network.app.id, zcp_network.app_secondary.id]
  default_network  = zcp_network.app.id
  storage_category = "nvme"
  ssh_key          = "deploy-key"
}
```

## Import

Create-only attributes the API does not return are supplied positionally in the
import ID (slash-separated, trailing/empty segments allowed). `networks` does
not fit this scheme as a single value, so it is a trailing comma-separated
segment of its own:

```shell
terraform import zcp_instance.web '<slug>/<cloud_provider>/<region>/<template>[/<plan>/<billing_cycle>/<project>/<ssh_key>/<network>/<network_plan>/<storage_category>/<network_type>/<vr_plan>/<default_network>/<networks>]'
```

`<slug>/<cloud_provider>/<region>/<template>` are required. `name`, `state` and
the IPs come from the subsequent read.

## Schema

### Required

- `name` (String) Display name of the instance. Updated in place. The hostname
  is set from this value at creation and is not changed afterward.
- `cloud_provider` (String) Cloud provider slug. Use
  `data.zcp_region.<name>.cloud_provider`. Changing this forces replacement.
- `region` (String) Region slug (e.g. `yow-1`). Changing this forces
  replacement.
- `template` (String) Template (OS image) slug. See `data.zcp_template`.
  Changing this forces replacement.
- `plan` (String) Compute plan slug. Run `zcp plan vm` to list values. Updated
  in place (resize): the provider stops the instance, changes the offering, and
  restarts it to its prior running/stopped state.
- `billing_cycle` (String) Billing cycle (`hourly` or `monthly`). Updated in
  place together with `plan`.

### Optional

- `project` (String) Project slug. Inherits from the provider `default_project`
  if omitted. Changing this forces replacement.
- `ssh_key` (String) Name of an existing SSH key to attach for login (see
  `zcp_ssh_key`). Changing this forces replacement.
- `network` (String) Slug of an existing `zcp_network` to attach the instance
  to. Mutually exclusive with `network_plan` and with `networks`. Preferred: you
  manage the network, so `terraform destroy` leaves nothing behind. Changing
  this forces replacement.
- `network_plan` (String) Network plan slug (e.g. `pnet-yow`) used to
  auto-create an isolated network. Mutually exclusive with `network` and with
  `networks`. Not allowed when `network_type` is `Vpc`. Terraform does not
  manage the auto-created network and destroy does not remove it. Run
  `zcp plan network` to list values. Changing this forces replacement.
- `network_type` (String) Network type: `Isolated`, `L2`, or `Vpc`. Treated as
  `Isolated` when omitted. It is a plain optional attribute, not computed, so
  omitting it never writes a value to state and never plans a replacement for an
  instance created before this attribute existed. `Isolated` and `L2` accept
  `network_plan`, `network`, or `networks`. `Vpc` accepts `vr_plan` or
  `networks`, but not `network_plan`. `L2` requires `assign_public_ip = false`.
  Changing this forces replacement.
- `vr_plan` (String) Virtual router plan slug used when `network_type` is `Vpc`.
  Not allowed for `Isolated` or `L2`. Changing this forces replacement.
- `networks` (List of String) Slugs of existing networks to attach the instance
  to. Supersedes `network`. Setting both is a conflict. An empty list counts as
  not set. With more than one entry, `default_network` is required and must be
  one of them. Changing this forces replacement.
- `default_network` (String) Slug of the network in `networks` to treat as the
  instance's default network. Required when `networks` has more than one entry.
  Must match `network` when `network` is set instead of `networks`. Changing
  this forces replacement.
- `assign_public_ip` (Boolean) Whether to assign a public IP. Defaults to
  `true`. Set to `false` for a private-only instance. Must be `false` when
  `network_type` is `L2`. The platform rejects a public IP on an `L2` network.
  When `true`, destroy releases the auto-assigned IP (see the note above).
  Changing this forces replacement.
- `storage_category` (String) Storage category slug. Region-specific:
  `nvme`/`hdd-storage` in yow-1, `pro-nvme`/`premium-ssd` in yul-1. Required by
  the public API. Changing this forces replacement.
- `user_data` (String) Startup script content (cloud-init / bash). Updated in
  place via change-startup-script (takes effect on next boot).
- `tags` (Map of String) Key/value tags applied via tag-create / tag-delete.
  **Write-only:** the API does not return tags on read, so they are tracked in
  state but not refreshed (no drift detection) and are not populated on import.
- `timeouts` (Block) Configurable `create`, `update`, and `delete` timeouts.
  Create defaults to 20m.

### Read-Only

- `id` (String) Instance slug (unique identifier).
- `slug` (String) Instance slug (same value as `id`).
- `state` (String) Current runtime state of the instance (e.g. `Running`,
  `Stopped`), reported for information only. Terraform does not reconcile or
  manage power state.
- `private_ip` (String) Private IP of the instance's default network.
- `public_ip` (String) Public IP of the instance, if assigned. When the address
  belongs to the network (source-NAT), the provider resolves it from the account
  IP list, since the platform leaves it off the VM object.
