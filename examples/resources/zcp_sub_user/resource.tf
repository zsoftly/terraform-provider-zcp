resource "zcp_sub_user" "alice" {
  name     = "Alice"
  email    = "alice@example.com"
  password = var.alice_password
  role     = zcp_role.ops.id
  projects = [zcp_project.staging.id]
}
