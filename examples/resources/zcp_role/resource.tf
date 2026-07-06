data "zcp_permissions" "compute" {
  category = "compute"
}

resource "zcp_role" "ops" {
  name        = "ops"
  description = "Operations team"
  permissions = data.zcp_permissions.compute.permissions[*].slug
}
