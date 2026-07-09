# DNS is account-level: the dedicated DNS provider and its single `default`
# region are applied automatically, so only the name is needed.
resource "zcp_dns_domain" "example" {
  name = "example.com"
}
