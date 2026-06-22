# cloud_provider is read from the region data source — no hardcoding.
data "zcp_region" "yow" {
  slug = "yow-1"
}

# An SSH key to attach for login (see zcp_ssh_key).
resource "zcp_ssh_key" "deploy" {
  name       = "deploy-key"
  public_key = file("~/.ssh/id_ed25519.pub")
  region     = data.zcp_region.yow.slug
}

# ── Basic instance ────────────────────────────────────────────────────────────
# Create blocks until the instance reaches the Running state, at which point
# private_ip and public_ip are populated in state.
#
# Changing `plan` later resizes the instance: the provider transparently stops
# it, changes the compute offering, and restarts it — no power management needed
# (same as changing an aws_instance instance_type).
#
# On the public ZCP platform, network_plan and storage_category are required by
# the API even though they are schema-optional (private-cloud configs may differ).
resource "zcp_instance" "web" {
  name             = "web-01"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  template         = "ubuntu-2404-lts"
  plan             = "ca1hxs"
  billing_cycle    = "hourly"
  network_plan     = "pnet-yow"
  storage_category = "nvme"
  ssh_key          = zcp_ssh_key.deploy.name
}

# ── Instance with optional extras: user data and tags ─────────────────────────
# tags are optional. Terraform does not manage the instance's power state — the
# running/stopped status is reported read-only in `state`.
resource "zcp_instance" "app" {
  name             = "app-01"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  template         = "ubuntu-2404-lts"
  plan             = "ci1hm"
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
#   e.g. terraform import zcp_instance.web 'web-01-abc/nimbo/yow-1/ubuntu-24-04'
