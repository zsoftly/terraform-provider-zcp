# cloud_provider is read from the region data source — no hardcoding.
data "zcp_region" "yow" {
  slug = "yow-1"
}

# ── Custom-size volume (recommended) ──────────────────────────────────────────
# Provide exactly one of `plan` or `size`. Size-based creation is the reliable
# path today (see the provider docs note on plan-based creation).
resource "zcp_volume" "data" {
  name             = "data-vol"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  billing_cycle    = "hourly"
  storage_category = "nvme"
  size             = 20
}

# ── Custom-size volume attached to an instance on creation ────────────────────
resource "zcp_instance" "db" {
  name           = "db-01"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
  template       = "ubuntu-24-04"
  plan           = "ci1.medium"
  billing_cycle  = "hourly"
}

resource "zcp_volume" "db_data" {
  name             = "db-data"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  billing_cycle    = "hourly"
  storage_category = "nvme"
  size             = 100 # GB — mutually exclusive with `plan`
  vm               = zcp_instance.db.id
}

# ── Outputs ───────────────────────────────────────────────────────────────────
output "data_volume_slug" {
  description = "Slug of the data volume."
  value       = zcp_volume.data.slug
}

# ── Import ────────────────────────────────────────────────────────────────────
# terraform import zcp_volume.data \
#   '<slug>/<cloud_provider>/<region>/<billing_cycle>/<storage_category>[/<plan>/<project>/<vm>]'
#   e.g. terraform import zcp_volume.data 'data-vol-abc/nimbo/yow-1/hourly/nvme/b1g1'
# `size` cannot be imported via the ID; set it in config for size-based volumes.
