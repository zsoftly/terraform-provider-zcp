---
page_title: "zcp_ip_address Resource"
description: |-
  Allocate and manage ZCP public IP addresses.
---

# zcp_ip_address

Allocates a public IP address into a VPC or network. The API has no update endpoint for IP addresses, so every change forces replacement. Bind the IP to an instance with `zcp_ip_association`, then open ports with `zcp_firewall_rule` or `zcp_port_forward`.

## Example Usage

```terraform
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

## Import

Import using the IP address slug:

```shell
terraform import zcp_ip_address.web 1036521143
```

## Schema

### Required

- `plan` (String) Plan slug for the IP address (e.g. `public-ip-1`). Changing this forces replacement.
- `billing_cycle` (String) Billing cycle (e.g. `hourly`, `monthly`). Changing this forces replacement.

### Optional

- `vpc` (String) VPC slug to associate with the IP. At least one of `vpc` or `network` must be set. Changing this forces replacement.
- `network` (String) Network slug to associate with the IP. At least one of `vpc` or `network` must be set. Changing this forces replacement.
- `project` (String) Project slug. Inherits from the provider `default_project` if omitted. Changing this forces replacement.

### Read-Only

- `id` (String) IP address slug.
- `ip_address` (String) The allocated public IP address.
- `type` (String) IP address type (e.g. `Public`).
