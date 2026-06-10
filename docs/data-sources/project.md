---
page_title: "zcp_project Data Source"
description: |-
  Look up a ZCP project by slug.
---

# zcp_project

Look up a ZCP project by slug and expose its attributes as read-only values.

## Example Usage

```terraform
data "zcp_project" "default" {
  slug = "default"
}

output "project_name" {
  value = data.zcp_project.default.name
}
```

## Schema

### Required

- `slug` (String) Unique project slug.

### Read-Only

- `id` (String) Project ID.
- `name` (String) Project display name.
- `description` (String) Project description.
