data "zcp_instance" "web" {
  slug = "vm1-web"
}

output "root_volume" {
  value = data.zcp_instance.web.root_volume
}
