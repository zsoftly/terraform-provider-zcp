data "zcp_region" "yow" {
  slug = "yow-1"
}

data "zcp_volume" "data_disk" {
  slug   = "data-1234"
  region = data.zcp_region.yow.slug
}
