resource "zcp_autoscale_group" "web" {
  name           = "web-asg"
  plan           = "ci1xs"
  template       = "ubuntu-24"
  min_instances  = 1
  max_instances  = 5
  zone           = "yow-zone-1"
  network        = zcp_network.prod.id
  cloud_provider = data.zcp_region.yow.cloud_provider
  region         = data.zcp_region.yow.slug
}
