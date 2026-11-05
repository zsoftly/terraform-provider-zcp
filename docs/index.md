---
page_title: "ZCP Provider"
description: |-
  Use the ZCP provider to manage ZSoftly Cloud Platform resources.
---

# ZCP Provider

The ZCP provider manages [ZSoftly Cloud Platform](https://zcp.zsoftly.ca)
resources: compute instances, networks, Kubernetes clusters, object storage, and
more.

## Example Usage

**OpenTofu (primary):**

```terraform
terraform {
  required_providers {
    zcp = {
      source  = "registry.opentofu.org/zsoftly/zcp"
      version = "~> 1.3.0"
    }
  }
}

provider "zcp" {
  bearer_token    = var.zcp_bearer_token
  default_project = "default"
}
```

**Terraform:**

```terraform
terraform {
  required_providers {
    zcp = {
      source  = "registry.terraform.io/zsoftly/zcp"
      version = "~> 1.3.0"
    }
  }
}

provider "zcp" {
  bearer_token    = var.zcp_bearer_token
  default_project = "default"
}
```

## Local Development

The examples use the provider v1.3.x series, which includes CLI profile
authentication, separate Kubernetes node and storage plans, VM backup schedules,
plan tags, and block storage snapshot schedules.

During development, use `dev_overrides` instead of a registry install. A single
binary works for both CLIs via config.

```hcl
# ~/.tofurc (OpenTofu)
provider_installation {
  dev_overrides {
    "registry.opentofu.org/zsoftly/zcp" = "/path/to/terraform-provider-zcp"
  }
  direct {}
}
```

```hcl
# ~/.terraformrc (Terraform)
provider_installation {
  dev_overrides {
    "registry.terraform.io/zsoftly/zcp" = "/path/to/terraform-provider-zcp"
  }
  direct {}
}
```

For non-dev_overrides local installs, the binary must be compiled with the
correct registry address. The Makefile handles this:

- `make install` builds with `registry.opentofu.org` address, installs to
  `~/.opentofu/plugins/`
- `make install-terraform` builds with `registry.terraform.io` address, installs
  to `~/.terraform.d/plugins/`
- `make dev-install` rebuilds in-place for dev_overrides workflows

## Authentication

Set `ZCP_BEARER_TOKEN` in your environment. No HCL attribute is required. If
neither `bearer_token` nor `ZCP_BEARER_TOKEN` is set, the provider uses the ZCP
CLI profile selected by `profile` or `ZCP_PROFILE`. When neither selector is
set, the provider uses the active CLI profile. The provider uses an available
profile's API URL and default project. The profile's bearer token is used only
after `bearer_token` and `ZCP_BEARER_TOKEN`. Optionally set `ZCP_API_URL` to
override the endpoint and `ZCP_PROJECT` to set a default project. Get the bearer
token from the [ZCP console](https://cloud.zcp.zsoftly.ca) under **Account → API
Keys**.

```bash
export ZCP_BEARER_TOKEN="<your-token>"
export ZCP_PROJECT="default"   # optional
```

## Schema

### Optional

- `bearer_token` (String, Sensitive) ZCP API bearer token. Supply the token
  through this attribute or the `ZCP_BEARER_TOKEN` environment variable. It
  takes precedence over all other credential sources.
- `api_url` (String) ZCP API base URL. Defaults to
  `https://api.zcp.zsoftly.ca/api`. May also be set via `ZCP_API_URL`.
- `default_project` (String) Default project slug applied to resources that omit
  a project. May also be set via `ZCP_PROJECT`.
- `profile` (String) ZCP CLI profile whose API URL and default project apply.
  The profile's bearer token is used after `bearer_token` and
  `ZCP_BEARER_TOKEN`. May also be set via `ZCP_PROFILE`. An explicitly selected
  profile must exist. The provider never switches credential sources after the
  API rejects one.
