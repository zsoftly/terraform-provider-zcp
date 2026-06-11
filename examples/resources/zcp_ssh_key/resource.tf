# ── Example 1: single deploy key read from disk ───────────────────────────────
resource "zcp_ssh_key" "deploy" {
  name       = "deploy-key"
  public_key = file("~/.ssh/id_ed25519.pub")
}

# ── Example 2: key material inline (e.g. from a variable or secret store) ─────
variable "ci_public_key" {
  description = "CI runner public key."
  type        = string
  sensitive   = true
}

resource "zcp_ssh_key" "ci_runner" {
  name       = "ci-runner"
  public_key = var.ci_public_key
}

# ── Example 3: multiple keys managed as a map ─────────────────────────────────
variable "team_keys" {
  description = "Map of engineer name → OpenSSH public key."
  type        = map(string)
  default     = {}
}

resource "zcp_ssh_key" "team" {
  for_each   = var.team_keys
  name       = each.key
  public_key = each.value
}

# ── Outputs ───────────────────────────────────────────────────────────────────
output "deploy_key_id" {
  description = "Slug of the deploy SSH key (reference this when creating VMs)."
  value       = zcp_ssh_key.deploy.id
}

output "team_key_ids" {
  description = "Map of engineer name → SSH key slug."
  value       = { for k, v in zcp_ssh_key.team : k => v.id }
}
