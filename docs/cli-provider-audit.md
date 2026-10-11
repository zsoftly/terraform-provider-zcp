---
page_title: "zcp-cli v0.0.31 to v1.0.0 Provider Audit"
description: |-
  Maps released zcp-cli v1.0.0 changes to Terraform provider v1.0.0 behavior.
---

# zcp-cli v0.0.31 to v1.0.0 Provider Audit

This tracker compares the published
[v0.0.31 to v1.0.0 range](https://github.com/zsoftly/zcp-cli/compare/v0.0.31...v1.0.0).
Provider v1.0.0 resolves the published zcp-cli v1.0.0 SDK with `GOWORK=off`. The
table records the provider changes and their verification scope.

| SDK or CLI change                                                                                                              | Provider disposition                                                                                                                                                                                                                                  |
| ------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Kubernetes separate control-plane, worker, storage plans, root disk capacity, CSI, response details, and autoscaling endpoints | Implemented in `zcp_kubernetes_cluster`. Fixed plans and root size replace clusters because no role-specific update API exists. Network and root-volume slugs are computed. Kubeconfig is excluded from state.                                        |
| Kubernetes autoscaling commands and bounds                                                                                     | Implemented. A staging workload did not trigger scale-out, and scale or autoscale calls recreated root-volume subscriptions. The provider reports accepted bounds and does not promise a scale-out or stable subscription state.                      |
| Kubernetes deletion through billing cancellation                                                                               | Implemented. Destroy validates lifecycle state, submits immediate service cancellation, and polls for absence.                                                                                                                                        |
| Compute plan and root-disk handling                                                                                            | Implemented by `zcp_instance`. New instances require a named compute plan, `blockstorage_plan`, and `root_disk_size`; custom CPU, memory, and disk inputs are rejected. Legacy custom-plan state remains readable and destroyable.                    |
| DNS pagination and FindBySlug                                                                                                  | Implemented by `zcp_dns_domain`. Reads locate domains through the paginated SDK lookup and remove state only when no matching slug exists.                                                                                                            |
| Scheduler API                                                                                                                  | Implemented by `zcp_vm_backup_schedule`. It creates, reads, updates, pauses, resumes, deletes, imports, and scopes VM backup policies. The resource verifies the VM before create and verifies returned policy identity, action, region, and project. |
| VM backup restore and schedule run-now                                                                                         | CLI operational behavior only. A restore or an ad hoc run does not describe durable Terraform state.                                                                                                                                                  |
| CLI validation, help text, printer changes, and smoke workflows                                                                | CLI-only behavior. Terraform uses framework configuration, diagnostics, and state.                                                                                                                                                                    |
| Backup schedule intervals                                                                                                      | `zcp_vm_backup_schedule` supports `hourly`, `dailyAt`, `weeklyOn`, and `monthlyOn`. `everyOtherDay` is rejected before a request because the platform persists it and then reports an error.                                                          |
| Plan tags                                                                                                                      | Implemented by `data.zcp_plan.tag`, using the SDK's `tag` value with its legacy label fallback.                                                                                                                                                       |

## Verification status

Local unit tests, `GOWORK=off` vet, and a provider build cover the SDK
integration. On 2026-10-11, a local provider built with `GOWORK=off` against the
published SDK v1.0.0 completed staging catalog reads for compute, storage, and
Kubernetes.

A named-plan VM with a 20 GB root disk created in 72 seconds and refreshed to
zero diff. A backup schedule using the provider default project created in 9
seconds; retention, timezone, and pause changes updated in place in 9 seconds.
Weekly, monthly, hourly, and daily schedules resumed in place and refreshed to
zero diff. Import refreshed to zero diff. Terraform destroyed the schedule in 2
seconds, VM in 26 seconds, and network in 18 seconds, leaving state empty.

A one-control-plane, one-worker Kubernetes cluster using separate plans, a 100
GB root disk, and a CSI request reached `Running` in 264 seconds. Its network
and two root volumes were returned, refresh was zero diff, and a fresh 14-field
split-plan import with explicit `ha = false` refreshed to zero diff.

A manual Kubernetes worker resize from one to two completed in 158 seconds and
refreshed to zero diff. Autoscaling enable completed in 24 seconds, and a
maximum-worker update from two to three completed in 20 seconds. Both refreshed
to zero diff. The final local provider disabled autoscaling and reduced workers
from two to one in 85 seconds, followed by a zero-diff plan. It disabled
autoscaling before resizing. The cluster deleted in 71 seconds and its SSH key
in 1 second; nodes and root volumes were absent afterward. Cluster cancellation
left an empty network and an active subscription. Terraform imported that
residual network into an isolated cleanup fixture and destroyed it in 1 second.
A final paginated inventory found no test matches, and all three Terraform
states had no managed resources. The validation did not run a backup, restore a
backup, trigger automatic scale-out, or run a CSI workload.

## Historical live evidence

The 2026-10-09 live checks for v0.3.0 remain historical evidence for ACL,
object-storage, custom-instance, and load-balancer behavior. They do not verify
this v1.0.0 change set.
