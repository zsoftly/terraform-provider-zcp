---
page_title: "ZCP Provider"
description: |-
  Use the ZCP provider to manage ZSoftly Cloud Platform resources.
---

# ZCP Provider

The ZCP provider manages [ZSoftly Cloud Platform](https://zcp.zsoftly.ca) resources: compute instances, networks, Kubernetes clusters, object storage, and more.

## Example Usage

```terraform
provider "zcp" {
  bearer_token    = var.zcp_bearer_token
  default_project = "default"
}
```

## Authentication

Set `ZCP_BEARER_TOKEN` in your environment — no HCL attribute is required. Optionally set `ZCP_API_URL` to override the endpoint and `ZCP_PROJECT` to set a default project. The bearer token is obtained from the ZCP dashboard under **Account → API Keys**.

```bash
export ZCP_BEARER_TOKEN="<your-token>"
export ZCP_PROJECT="default"   # optional
```

## Schema

### Optional

- `bearer_token` (String, Sensitive) ZCP API bearer token. May also be set via `ZCP_BEARER_TOKEN`. Required — either the attribute or the environment variable must be set.
- `api_url` (String) ZCP API base URL. Defaults to `https://api.zcp.zsoftly.ca/api`. May also be set via `ZCP_API_URL`.
- `default_project` (String) Default project slug applied to resources that omit a project. May also be set via `ZCP_PROJECT`.
