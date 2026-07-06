---
page_title: "zcp_project Resource"
description: |-
  Create and manage ZCP projects.
---

# zcp_project

Manages a ZCP project. `name`, `description`, and `purpose` update in place. Deleting a project requires it to be empty of services.

Adding users to a project (`zcp project user add`) has no removal API, so project membership stays a CLI operation.

## Example Usage

```terraform
resource "zcp_project" "staging" {
  name        = "staging"
  description = "Staging workloads"
}

resource "zcp_instance" "web" {
  # ...
  project = zcp_project.staging.id
}
```

## Import

Import using the project slug:

```shell
terraform import zcp_project.staging staging-p1
```

## Schema

### Required

- `name` (String) Project display name. Updated in place.

### Optional

- `description` (String) Human-readable description. Updated in place when set. Removing it from configuration preserves the remote value because the API cannot clear it on update.
- `purpose` (String) Project purpose. Updated in place when set. Removing it from configuration preserves the remote value because the API cannot clear it on update.
- `icon` (String) Project icon identifier (see `zcp project icon list`). Defaults to `cloud-13`. Set at create time only; changing it forces replacement.

### Read-Only

- `id` (String) Project slug.
