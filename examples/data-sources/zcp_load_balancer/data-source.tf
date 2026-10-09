variable "load_balancer_slug" {
  type        = string
  description = "Slug of an existing load balancer."
}

data "zcp_load_balancer" "existing" {
  slug = var.load_balancer_slug
}

output "load_balancer_rule_ids" {
  value = [for rule in data.zcp_load_balancer.existing.rules : rule.id]
}
