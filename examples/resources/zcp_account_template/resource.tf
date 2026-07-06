# Capture a golden image from a prepared instance.
resource "zcp_account_template" "golden_web" {
  name            = "golden-web"
  virtual_machine = zcp_instance.web.id
  cloud_provider  = data.zcp_region.yow.cloud_provider
  region          = data.zcp_region.yow.slug
  billing_cycle   = "monthly"
}
