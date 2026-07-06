# name is the relative label; the backend appends the zone.
resource "zcp_dns_record" "www" {
  domain  = zcp_dns_domain.example.id
  name    = "www"
  type    = "A"
  content = zcp_instance.web.public_ip
  ttl     = 3600
}

# Apex TXT record.
resource "zcp_dns_record" "spf" {
  domain  = zcp_dns_domain.example.id
  name    = "@"
  type    = "TXT"
  content = "\"v=spf1 -all\""
  ttl     = 3600
}
