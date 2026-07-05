# cloud_provider is read from the region data source — no hardcoding.
data "zcp_region" "yow" {
  slug = "yow-1"
}

# An SSH key to attach for login (see zcp_ssh_key).
resource "zcp_ssh_key" "deploy" {
  name       = "deploy-key"
  public_key = file(pathexpand("~/.ssh/id_ed25519.pub"))
  region     = data.zcp_region.yow.slug
}

# ── Recommended: attach the instance to a network you manage ──────────────────
# Create the network as a first-class resource and attach the instance to it with
# `network`. Both are then cleaned up by `terraform destroy` with no orphans.
# (Using `network_plan` instead auto-creates a network Terraform doesn't track.)
#
# Create blocks until the instance reaches Running, at which point private_ip and
# public_ip are populated. Changing `plan` later resizes in place (the provider
# stops, changes the offering, and restarts — like changing an aws_instance type).
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
  ssh_key          = zcp_ssh_key.deploy.name
  assign_public_ip = true # set false for a private-only instance
}

# ── Convenience: auto-create a network with network_plan ──────────────────────
# network_plan auto-creates an isolated network for the instance. It is simpler,
# but the network is NOT managed by Terraform and is left behind on destroy —
# prefer the `network` attribute above. tags are optional; power state is not
# managed by Terraform (the running/stopped status is read-only in `state`).
resource "zcp_instance" "app" {
  name             = "app-01"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  template         = "ubuntu-2404-lts"
  plan             = "ci1m"
  billing_cycle    = "hourly"
  network_plan     = "pnet-yow"
  storage_category = "nvme"
  ssh_key          = zcp_ssh_key.deploy.name

  user_data = <<-EOT
    #cloud-config
    package_update: true
    packages:
      - nginx
  EOT

  tags = {
    Environment = "production"
    Team        = "platform"
  }
}

# ── Outputs ───────────────────────────────────────────────────────────────────
output "web_private_ip" {
  description = "Private IP of the web instance."
  value       = zcp_instance.web.private_ip
}

output "web_public_ip" {
  description = "Public IP of the web instance, if assigned."
  value       = zcp_instance.web.public_ip
}

# ── Import ────────────────────────────────────────────────────────────────────
# Create-only attributes the API does not echo back are supplied positionally:
# terraform import zcp_instance.web \
#   '<slug>/<cloud_provider>/<region>/<template>[/<plan>/<billing_cycle>/<project>/<ssh_key>/<network_plan>/<storage_category>]'
#   e.g. terraform import zcp_instance.web 'web-01-abc/nimbo/yow-1/ubuntu-2404-lts'
