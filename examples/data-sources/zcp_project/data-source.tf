data "zcp_project" "default" {
  slug = "default"
}

output "project_name" {
  value = data.zcp_project.default.name
}
