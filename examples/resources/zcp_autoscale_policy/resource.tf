# Scale up by one instance when CPU stays above 80% for 5 minutes.
resource "zcp_autoscale_policy" "cpu_high" {
  autoscale_group = zcp_autoscale_group.web.id
  name            = "cpu-high"
  metric          = "cpu"
  operator        = "GT"
  threshold       = 80
  duration        = 300
  scale_amount    = 1
}
