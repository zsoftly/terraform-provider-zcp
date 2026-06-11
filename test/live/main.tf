data "zcp_region" "yow" {
  slug = "yow-1"
}

data "zcp_region" "yul" {
  slug = "yul-1"
}

# ── VPCs ───────────────────────────────────────────────────────────────────────

resource "zcp_vpc" "yow_test" {
  name             = "tf-test-yow"
  cloud_provider   = data.zcp_region.yow.cloud_provider
  region           = data.zcp_region.yow.slug
  cidr             = "10.1.0.1"
  size             = "24"
  type             = "Vpc"
  billing_cycle    = "hourly"
  storage_category = "nvme"
  plan             = "virtual-private-cloud-vpc-1"
  description      = "Terraform provider live test — YOW"
}

resource "zcp_vpc" "yul_test" {
  name             = "tf-test-yul"
  cloud_provider   = data.zcp_region.yul.cloud_provider
  region           = data.zcp_region.yul.slug
  cidr             = "10.2.0.1"
  size             = "24"
  type             = "Vpc"
  billing_cycle    = "hourly"
  storage_category = "nvme"
  plan             = "virtual-private-cloud-vpc-1"
  description      = "Terraform provider live test — YUL"
}

# ── VPN User ──────────────────────────────────────────────────────────────────

resource "zcp_vpn_user" "yow_test" {
  username       = "tf-test-vpn-user"
  password       = "ChangeMe123"
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
}

# ── VPN Customer Gateway ──────────────────────────────────────────────────────

resource "zcp_vpn_customer_gateway" "test" {
  name                = "tf-test-cgw"
  gateway             = "203.0.113.1"
  cidr_list           = "192.168.100.0/24"
  ipsec_psk           = "s3cr3t-psk"
  ike_policy          = "aes128-sha1-dh5"
  esp_policy          = "aes128-sha1"
  ike_lifetime        = "86400"
  esp_lifetime        = "3600"
  ike_version         = "ikev2"
  ike_encryption      = "aes128"
  ike_hash            = "sha1"
  ike_dh              = "modp1024"
  esp_encryption      = "aes128"
  esp_hash            = "sha1"
  esp_dh              = "modp1024"
  esp_pfs             = "modp1024"
  force_encapsulation = false
  split_connections   = false
  dead_peer_detection = false
  cloud_provider      = data.zcp_region.yow.cloud_provider
  region              = data.zcp_region.yow.slug
}

# ── VPC VPN Gateway ───────────────────────────────────────────────────────────

resource "zcp_vpc_vpn_gateway" "yow_test" {
  vpc = zcp_vpc.yow_test.id
}

# Blocked: SSH key creation hits a CMP PHP null-pointer bug on the backend.
# resource "zcp_ssh_key" "test" { ... }

# Blocked: Network categories not configured in CMP for yow-1/yul-1 (HTTP 403).
# resource "zcp_network" "yow_test" { ... }
# resource "zcp_network" "yul_test" { ... }

# Blocked: IP address allocation requires a VPC with at least one network.
# resource "zcp_ip_address" "yow_test" { ... }
# resource "zcp_port_forward" "http" { ... }
# resource "zcp_firewall_rule" "allow_http" { ... }

# ── Outputs ───────────────────────────────────────────────────────────────────

output "yow_vpc_id" {
  value = zcp_vpc.yow_test.id
}

output "yow_vpc_status" {
  value = zcp_vpc.yow_test.status
}

output "yul_vpc_id" {
  value = zcp_vpc.yul_test.id
}

output "yul_vpc_status" {
  value = zcp_vpc.yul_test.status
}

output "yow_vpn_user_id" {
  value = zcp_vpn_user.yow_test.id
}

output "test_cgw_id" {
  value = zcp_vpn_customer_gateway.test.id
}

output "yow_vpc_vpn_gw_id" {
  value = zcp_vpc_vpn_gateway.yow_test.id
}
