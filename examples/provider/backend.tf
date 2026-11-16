
terraform {
  required_version = ">= 1.0"
  required_providers {
    zcp = {
      source  = "registry.terraform.io/zsoftly/zcp"
      version = "~> 1.3.0"
    }
  }

  backend "s3" {
    bucket = "example-bucket"
    key    = "zsoftly/terraform-provider-zcp/examples/provider/backend.tfstate"
    region = "us-east-1" # placeholder, not validated

    endpoints = {
      s3 = "https://objects.yow.zcp.zsoftly.ca"
    }
    use_lockfile = true # native S3 lockfile; ZCP supports conditional writes

    # skip AWS-specific backend validation (ZCP has no STS/IAM/IMDS)
    skip_credentials_validation = true
    skip_requesting_account_id  = true
    skip_metadata_api_check     = true
    skip_region_validation      = true

    access_key = "access_123"             # ZCP_S3_ACCESS_KEY — recommend using a backend config file
    secret_key = "secret123456789abcdefg" # ZCP_S3_SECRET_KEY — recommend using a backend config file

    use_path_style = true # ZCP uses path-style URLs (host/bucket/key)

  }
}


provider "zcp" {
  # bearer_token    =  var.zcp_bearer_token  # or set ZCP_BEARER_TOKEN (recommended)
  # api_url         = "https://api.zcp.zsoftly.ca/api"  # or set ZCP_API_URL; this is the default
  # default_project = "default"             # or set ZCP_PROJECT
}