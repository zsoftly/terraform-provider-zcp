data "zcp_region" "yow" {
  slug = "yow-1"
}

output "region_name" {
  value = data.zcp_region.yow.name
}

output "region_country" {
  value = data.zcp_region.yow.country
}
