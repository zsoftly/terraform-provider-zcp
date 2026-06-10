# Contributing

Thank you for your interest in terraform-provider-zcp.

## Reporting Issues

Please use [GitHub Issues](https://github.com/zsoftly/terraform-provider-zcp/issues) to report bugs or request features.

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

- Go 1.25+
- Terraform 1.0+ or OpenTofu 1.6+

### Local Dev Override

To load the provider from your local build instead of the registry, add a `dev_overrides` block to `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "zsoftly/zcp" = "/Users/<you>/go/bin"
  }
  direct {}
}
```

Then run `make install` to build and place the binary, and `terraform init` in any example directory will pick it up.

### zcp-cli Dependency

This provider depends on `github.com/zsoftly/zcp-cli` for shared API client types. During local development, `go.mod` contains a `replace` directive pointing to `../zcp-cli`. Before cutting a release, update `go.mod` to pin a tagged release of `zcp-cli` and remove the `replace` directive.

> **Note:** `zcp-cli` currently exposes its API clients under `internal/`, which restricts cross-module imports. A future ticket will move the relevant packages to `pkg/api/` to make them importable here.
