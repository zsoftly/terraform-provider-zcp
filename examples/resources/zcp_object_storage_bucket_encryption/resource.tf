terraform {
  required_providers {
    zcp = {
      source = "zsoftly/zcp"
    }
  }
}

provider "zcp" {}

variable "object_storage" {
  type        = string
  description = "Existing object storage slug in a region with encryption support."
}

variable "bucket" {
  type        = string
  description = "Existing bucket slug."
}

variable "kms_key_id" {
  type        = string
  description = "Operator-provisioned service-managed key ID, without slashes."
}

resource "zcp_object_storage_bucket_encryption" "assets" {
  object_storage = var.object_storage
  bucket         = var.bucket
  algorithm      = "aws:kms"
  kms_key_id     = var.kms_key_id
}
