data "zcp_region" "yow" {
  slug = "yow-1"
}

output "region_name" {
  value = data.zcp_region.yow.name
}

output "region_country" {
  value = data.zcp_region.yow.country
}

output "region_cloud_provider" {
  description = "Pass to cloud_provider on zcp_network / zcp_vpc resources."
  value       = data.zcp_region.yow.cloud_provider
}
