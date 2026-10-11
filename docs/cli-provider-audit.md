---
page_title: "zcp-cli v0.0.31 to v1.0.1 Provider Audit"
description: |-
  Maps released zcp-cli v1.0.1 changes to Terraform provider v1.0.0 behavior.
---

# zcp-cli v0.0.31 to v1.0.1 Provider Audit

This tracker compares the published
[v0.0.31 to v1.0.1 range](https://github.com/zsoftly/zcp-cli/compare/v0.0.31...v1.0.1).
Provider v1.0.0 resolves the published zcp-cli v1.0.1 SDK with `GOWORK=off`.

| SDK or CLI change                                                                                            | Provider disposition                                                                                                                                                                                                                                                                                                          |
| ------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Instance hostname change                                                                                     | `zcp_instance` uses `ChangeHostname` with the current name and requested hostname. It waits for the updated name and hostname plus a stable runtime state before later updates. A name change can restart a running instance or start a stopped instance.                                                                     |
| Instance deletion                                                                                            | `zcp_instance` uses service cancellation and waits for the instance to disappear. This preserves public-IP cleanup.                                                                                                                                                                                                           |
| Block storage snapshot pagination                                                                            | `zcp_volume_snapshot` uses the SDK snapshot service, which retrieves every page and returns an error if a later page fails.                                                                                                                                                                                                   |
| Kubernetes separate control-plane, worker, storage plans, root disk capacity, CSI, and autoscaling endpoints | `zcp_kubernetes_cluster` uses fixed plans, root size, and optional CSI. Changing these fields forces replacement because no role-specific update operation exists. Autoscaling records accepted worker bounds but does not wait for scale-out. Network and root-volume slugs are computed. Kubeconfig is excluded from state. |
| Kubernetes deletion through billing cancellation                                                             | Destroy validates lifecycle state, submits immediate service cancellation, and polls for absence.                                                                                                                                                                                                                             |
| Compute plan and root-disk handling                                                                          | `zcp_instance` requires a named compute plan, `blockstorage_plan`, and `root_disk_size` for new instances. Custom CPU, memory, and disk inputs are rejected. Legacy custom-plan state remains readable and destroyable.                                                                                                       |
| Volume tier and capacity create options                                                                      | `zcp_volume` supports a named `plan` with `size` to select capacity. It also preserves plan-only and size-only configuration.                                                                                                                                                                                                 |
| DNS pagination and slug lookup                                                                               | `zcp_dns_domain` reads domains through the paginated SDK lookup and removes state only when no matching slug exists.                                                                                                                                                                                                          |
| Scheduler API and backup intervals                                                                           | `zcp_vm_backup_schedule` creates, reads, updates, pauses, resumes, deletes, imports, and scopes policies. It accepts `hourly`, `dailyAt`, `weeklyOn`, and `monthlyOn`; `everyOtherDay` is rejected before a request.                                                                                                          |
| Plan tags                                                                                                    | `data.zcp_plan.tag` uses the SDK `tag` value with its legacy label fallback.                                                                                                                                                                                                                                                  |
| CLI-only operations                                                                                          | Backup restore and ad hoc schedule run operations do not describe durable Terraform state. CLI validation, help text, printer changes, and smoke workflows do not affect provider behavior.                                                                                                                                   |

## Verification status

Unit tests cover the SDK contract, asynchronous hostname updates, update error
handling, timeout handling, and stale state after a rename plus resize. The
provider also passes `GOWORK=off` vet, race tests, and a build against the
published SDK.
