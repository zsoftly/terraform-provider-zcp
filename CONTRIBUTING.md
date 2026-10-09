# Contributing

Thank you for your interest in terraform-provider-zcp.

## Reporting Issues

Please use
[GitHub Issues](https://github.com/zsoftly/terraform-provider-zcp/issues) to
report bugs or request features.

When filing a bug report, include:

- The provider version
- Your Terraform / OpenTofu version
- The exact configuration that reproduces the issue
- The expected vs. actual output
- Any relevant `TF_LOG=DEBUG` output

## Pull Requests

Before opening a pull request:

1. Open an issue first to discuss the change.
2. Fork the repository and create a feature branch.
3. Follow the existing code style (`make fmt` before committing).
4. Add or update tests for any changed behavior.
5. Run `make test-race` to confirm all tests pass.
6. Open a pull request with a clear description of the change.

## Development Setup

### Prerequisites

- Go 1.26.0 or later (the repository selects toolchain 1.26.9 in `go.mod`)
- Terraform 1.0+ or OpenTofu 1.6+

### Local Dev Override

To load the provider from your local build instead of the registry, add a
`dev_overrides` block to `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "zsoftly/zcp" = "/Users/<you>/go/bin"
  }
  direct {}
}
```

Then run `make install` to build and place the binary, and `terraform init` in
any example directory will pick it up.

### zcp-cli Dependency

This provider imports the tagged `github.com/zsoftly/zcp-cli` module and its
public `pkg/api` packages. Released builds validate the dependency outside a
workspace with `GOWORK=off`.

For local SDK development, add a sibling `zcp-cli` checkout to an ignored
`go.work` file. Do not use that workspace when validating a released provider
dependency.
