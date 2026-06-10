data "zcp_region" "yow" {
  slug = "yow-1"
}

data "zcp_template" "ubuntu" {
  slug        = "ubuntu-2204-lts"
  region_slug = data.zcp_region.yow.slug
}

output "template_id" {
  value = data.zcp_template.ubuntu.id
}
