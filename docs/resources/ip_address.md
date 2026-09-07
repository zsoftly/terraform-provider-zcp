---
page_title: "zcp_ip_address Resource"
description: |-
  Allocate and manage ZCP public IP addresses.
---

# zcp_ip_address

Allocates a public IP address into a VPC or network. The API has no update
endpoint for IP addresses, so every change forces replacement. Bind the IP to an
instance with `zcp_ip_association`, then open ports with `zcp_firewall_rule` or
`zcp_port_forward`.

~> **VPC allocation requires an existing network:** the API refuses to allocate
a public IP into a VPC until the VPC has at least one network (tier). It fails
with a 422 error stating there are no networks in the VPC. The `vpc` slug alone
gives Terraform no ordering information between the tier and the IP address. Add
an explicit `depends_on` on the tier, as shown in the VPC example below.

## Example Usage

```terraform
data "zcp_region" "yow" {
  slug = "yow-1"
}

resource "zcp_network" "prod" {
  name           = "prod-network"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  network_plan   = "inet-yow"
  billing_cycle  = "hourly"
}

resource "zcp_instance" "web" {
  name             = "web-01"
  template         = "ubuntu-2604-lts-1"
  plan             = "ca2sl"
  billing_cycle    = "hourly"
  network          = zcp_network.prod.id
  storage_category = "nvme"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
}

resource "zcp_ip_address" "web" {
  plan          = "public-ip-1"
  billing_cycle = "hourly"
  network       = zcp_network.prod.id
}

resource "zcp_ip_association" "web" {
  ip_address      = zcp_ip_address.web.id
  virtual_machine = zcp_instance.web.id
  network         = zcp_network.prod.id
}
```

### VPC Allocation

```terraform
resource "zcp_vpc" "main" {
  name           = "main-vpc"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  cidr           = "10.1.0.1"
  size           = "22"
  billing_cycle  = "hourly"
  plan           = "virtual-private-cloud-vpc"
}

resource "zcp_network" "tier" {
  name           = "web-tier"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  vpc            = zcp_vpc.main.id
  gateway        = "10.1.1.1"
  netmask        = "255.255.255.0"
  billing_cycle  = "hourly"
}

resource "zcp_ip_address" "vpc_ip" {
  plan          = "public-ip-1"
  billing_cycle = "hourly"
  vpc           = zcp_vpc.main.id

  # The vpc slug alone gives Terraform no ordering information: the tier must
  # exist before the API accepts an IP allocation into the VPC.
  depends_on = [zcp_network.tier]
}
```

## Import

Import using the IP address slug:

```shell
terraform import zcp_ip_address.web 1036521143
```

## Schema

### Required

- `plan` (String) Plan slug for the IP address (e.g. `public-ip-1`). Changing
  this forces replacement.
- `billing_cycle` (String) Billing cycle (e.g. `hourly`, `monthly`). Changing
  this forces replacement.

### Optional

- `vpc` (String) VPC slug to associate with the IP. At least one of `vpc` or
  `network` must be set. Changing this forces replacement.
- `network` (String) Network slug to associate with the IP. At least one of
  `vpc` or `network` must be set. Changing this forces replacement.
- `project` (String) Project slug. Inherits from the provider `default_project`
  if omitted. Changing this forces replacement.
- `timeouts` (Block) Configurable `create` and `delete` timeouts.

### Read-Only

- `id` (String) IP address slug.
- `ip_address` (String) The allocated public IP address.
- `type` (String) IP address type (e.g. `Public`).
