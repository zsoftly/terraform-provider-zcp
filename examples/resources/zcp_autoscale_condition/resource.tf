# Scale down by one instance when CPU stays below 20% for 10 minutes.
resource "zcp_autoscale_condition" "cpu_low" {
  autoscale_group = zcp_autoscale_group.web.id
  name            = "cpu-low"
  metric          = "cpu"
  operator        = "LT"
  threshold       = 20
  duration        = 600
  scale_amount    = 1
}
