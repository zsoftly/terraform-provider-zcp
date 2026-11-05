# terraform-provider-zcp v1.3.0 Release Notes

Provider v1.3.0 adds ZCP CLI profile authentication and builds against
`github.com/zsoftly/zcp-cli` v1.3.0. A provider can use a named CLI profile
through `profile` or `ZCP_PROFILE`. When neither is set, it uses the active CLI
profile.

The selected profile supplies the API URL and default project. Bearer-token
precedence is `bearer_token`, `ZCP_BEARER_TOKEN`, then the profile token. An
explicitly selected profile must exist. The provider reports unknown provider
configuration values instead of falling back to environment or profile values.
It does not try another credential when the selected credential is rejected.

```hcl
provider "zcp" {
  profile = "development"
}
```

## Upgrade

Set the provider constraint to `~> 1.3.0`, then install the release with the
same workflow used for prior versions:

Run `terraform init -upgrade` for Terraform or `tofu init -upgrade` for
OpenTofu.

Review the plan before applying changes.
