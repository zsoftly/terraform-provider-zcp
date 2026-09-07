# terraform-provider-zcp

[![Terraform Registry](https://img.shields.io/badge/Terraform%20Registry-zsoftly%2Fzcp-7B42BC?logo=terraform&logoColor=white)](https://registry.terraform.io/providers/zsoftly/zcp/latest)
[![OpenTofu Registry](https://img.shields.io/badge/OpenTofu%20Registry-zsoftly%2Fzcp-FFDA18?logo=opentofu&logoColor=black)](https://search.opentofu.org/provider/zsoftly/zcp/latest)

[![Release](https://img.shields.io/github/v/release/zsoftly/terraform-provider-zcp?logo=github&label=release&color=2ea44f)](https://github.com/zsoftly/terraform-provider-zcp/releases/latest)
[![CI](https://github.com/zsoftly/terraform-provider-zcp/actions/workflows/ci.yml/badge.svg)](https://github.com/zsoftly/terraform-provider-zcp/actions/workflows/ci.yml)

[![License: MIT](https://img.shields.io/github/license/zsoftly/terraform-provider-zcp?color=blue)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/zsoftly/terraform-provider-zcp?logo=go&logoColor=white)](go.mod)

Terraform and OpenTofu provider for the ZSoftly Cloud Platform (ZCP). It manages
compute, networking, Kubernetes, DNS, object storage, VPN, and account
governance through the ZCP API. Object storage bucket configuration uses the
store's S3-compatible gateway through the same SDK. The provider uses the
[zcp CLI](https://github.com/zsoftly/zcp-cli).

43 resources and 13 data sources. Full reference under [docs/](docs/), runnable
configurations under [examples/](examples/).

## Requirements

- Terraform 1.0 or later, or OpenTofu 1.6 or later (plugin protocol 6)
- A ZCP API bearer token

## Usage

```hcl
terraform {
  required_providers {
    zcp = {
      source  = "zsoftly/zcp"
      version = "~> 0.1"
    }
  }
}

provider "zcp" {
  default_project = "default-9"
}

data "zcp_region" "yul" {
  slug = "yul-1"
}

resource "zcp_network" "app" {
  name           = "app-network"
  cloud_provider = data.zcp_region.yul.cloud_provider
  region         = data.zcp_region.yul.slug
  network_plan   = "pnet-yul"
  billing_cycle  = "hourly"
}

resource "zcp_instance" "web" {
  name             = "web-01"
  template         = "ubuntu-2604-lts-1"
  plan             = "ca2sl"
  billing_cycle    = "hourly"
  network          = zcp_network.app.id
  storage_category = "premium-ssd"
  cloud_provider   = data.zcp_region.yul.cloud_provider
  region           = data.zcp_region.yul.slug
}
```

### Authentication

| Provider attribute | Environment variable | Purpose                                       |
| ------------------ | -------------------- | --------------------------------------------- |
| `bearer_token`     | `ZCP_BEARER_TOKEN`   | API bearer token (required)                   |
| `api_url`          | `ZCP_API_URL`        | API base URL, defaults to the public endpoint |
| `default_project`  | `ZCP_PROJECT`        | Project applied when a resource omits one     |

Generate the token in the [ZCP console](https://cloud.zcp.zsoftly.ca) under
**Account → API Keys**. Prefer the environment variable for the token so it
stays out of state and version control.

## Development

```bash
make build       # build for OpenTofu (default registry address)
make test        # go test -v -count=1 ./...
make test-race   # race detector
make fmt vet     # format and vet
```

Local runs against an unreleased build:

```bash
make dev-install   # prints the dev_overrides paths for .tofurc / .terraformrc
make install       # or install into the local OpenTofu plugin cache
```

For SDK development, point a `go.work` file at a sibling `zcp-cli` checkout and
keep it uncommitted. CI and releases build against the published
`github.com/zsoftly/zcp-cli` module.

## Releasing

Releases are tag driven. Pushing a `v*` tag runs GoReleaser through
[release.yml](.github/workflows/release.yml). The workflow builds the registry
artifact set (zip per platform, SHA256SUMS, GPG signature, registry manifest)
and publishes a GitHub release with [RELEASE_NOTES.md](RELEASE_NOTES.md) as the
body. Update `CHANGELOG.md` and `RELEASE_NOTES.md` before tagging.

## License

See [LICENSE](LICENSE).
