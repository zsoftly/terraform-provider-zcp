terraform {
  required_version = ">= 1.0"
  required_providers {
    zcp = {
      source  = "registry.opentofu.org/zsoftly/zcp"
      version = "~> 0.1"
    }
  }
}
