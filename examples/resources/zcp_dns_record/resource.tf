resource "zcp_dns_domain" "example" {
  name = "example.com"
}

# name is the relative label; the backend appends the zone.
resource "zcp_dns_record" "www" {
  domain  = zcp_dns_domain.example.id
  name    = "www"
  type    = "A"
  content = "203.0.113.10"
  ttl     = 3600
}

# Apex record: use "@" for the zone itself.
resource "zcp_dns_record" "spf" {
  domain  = zcp_dns_domain.example.id
  name    = "@"
  type    = "TXT"
  content = "\"v=spf1 -all\""
  ttl     = 3600
}

# MX record: priority is required and goes in its own argument.
resource "zcp_dns_record" "mx" {
  domain   = zcp_dns_domain.example.id
  name     = "@"
  type     = "MX"
  content  = "mail.example.com."
  priority = 10
  ttl      = 3600
}
