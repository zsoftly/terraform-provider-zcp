# terraform-provider-zcp

Terraform and OpenTofu provider for the ZSoftly Cloud Platform (ZCP). It manages
compute, networking, Kubernetes, DNS, object storage, VPN, and account
governance through the ZCP API, using the same SDK as the
[zcp CLI](https://github.com/zsoftly/zcp-cli).

38 resources and 12 data sources. Full reference under [docs/](docs/), runnable
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

resource "zcp_instance" "web" {
  name             = "web-01"
  template         = "ubuntu-2604-lts-1"
  plan             = "ca2sl"
  billing_cycle    = "hourly"
  network_plan     = "pnet-yul"
  storage_category = "premium-ssd"
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
