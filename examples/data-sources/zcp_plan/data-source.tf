data "zcp_plan" "small" {
  slug = "ci1xs"
}

output "plan_cpu"     { value = data.zcp_plan.small.cpu }
output "plan_memory"  { value = data.zcp_plan.small.memory_mb }
output "plan_monthly" { value = data.zcp_plan.small.monthly_price }
