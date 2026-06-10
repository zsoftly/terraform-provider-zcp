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
  endpoint = "https://api.zcp.zsoftly.ca/api"
  token    = var.zcp_token
}
```

## Authentication

Set `ZCP_TOKEN` (and optionally `ZCP_ENDPOINT`) in your environment, or configure them in the provider block. The token is a ZCP API bearer token obtained from the ZCP dashboard under **Account → API Keys**.

## Schema

### Optional

- `endpoint` (String) ZCP API endpoint URL. Defaults to `ZCP_ENDPOINT` environment variable.
- `token` (String, Sensitive) ZCP API bearer token. Defaults to `ZCP_TOKEN` environment variable.
