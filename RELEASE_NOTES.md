# terraform-provider-zcp Release Notes

## v1.0.0

Provider v1.0.0 uses the zcp-cli v1.0.0 SDK and adds separate compute and
storage configuration, Kubernetes autoscaling settings, and VM backup schedules.

- Added `zcp_vm_backup_schedule` for VM backup scheduler policies. It manages
  supported intervals, day and time, timezone, retention, pause state, import,
  and policy scope.
- DNS domain refresh now uses the SDK's paginated slug lookup. `data.zcp_plan`
  now exposes the decoded plan tag.
- New instances require a named compute plan, `blockstorage_plan`, and
  `root_disk_size`. The provider rejects custom CPU, memory, and disk inputs.
  Existing custom-plan state remains readable and destroyable.
- Kubernetes clusters use separate control-plane, worker, and root-volume plans,
  plus root disk capacity and autoscaling bounds. The legacy `plan` attribute
  remains for existing state only. Changing a cluster to split plans replaces
  it.
- Review the upgrade plan before applying. Existing resources are preserved in
  state, but creating or replacing an instance or cluster requires the new
  named-plan fields.
- `everyOtherDay` is rejected before a scheduler request because the platform
  persists the interval and then reports an error. Use `hourly`, `dailyAt`,
  `weeklyOn`, or `monthlyOn`.
- One-off VM backup restores and run-now operations remain CLI commands because
  they do not describe durable Terraform state.
- Staging testing did not demonstrate successful automatic scale-out. Scale and
  autoscale calls recreated root-volume subscriptions. The provider records
  accepted autoscaling bounds and does not promise a scale-out or stable
  subscription state.

### Live validation

On 2026-10-11, a local provider built with `GOWORK=off` against the published
SDK v1.0.0 completed staging catalog reads for compute, storage, and Kubernetes.
A new VM using a named compute plan and a 20 GB root disk created in 72 seconds
and then produced a zero-diff plan.

A VM backup schedule with the provider default project created in 9 seconds. Its
retention, timezone, and paused state updated in place in 9 seconds with the
same ID. Weekly, monthly, hourly, and daily schedules each resumed in place and
refreshed to zero diff. Import also refreshed to zero diff. Terraform destroyed
the schedule in 2 seconds, the VM in 26 seconds, and the network in 18 seconds.
The resulting state was empty.

A Kubernetes cluster with one control-plane node, one worker, separate plans, a
100 GB root disk, and a CSI request reached `Running` in 264 seconds. Its
network and two root volumes were returned, and refresh was zero diff. A fresh
14-field split-plan import with explicit `ha = false` also refreshed to zero
diff.

A manual Kubernetes worker resize from one to two completed in 158 seconds and
refreshed to zero diff. Autoscaling enable completed in 24 seconds, and a
maximum-worker update from two to three completed in 20 seconds. Both refreshed
to zero diff. The final local provider disabled autoscaling and reduced workers
from two to one in 85 seconds, followed by a zero-diff plan. It disabled
autoscaling before resizing. The cluster deleted in 71 seconds and its SSH key
in 1 second; nodes and root volumes were absent afterward. Cluster cancellation
left an empty network and an active subscription associated with it. Terraform
imported that residual network into an isolated cleanup fixture and destroyed it
in 1 second. A final paginated inventory found no test matches, and all three
Terraform states had no managed resources. This validation did not run a backup,
restore a backup, trigger an automatic scale-out, or run a CSI workload.

## v0.3.0

Historical release notes for v0.3.0 follow.

- ACL rule refreshes and post-create ID lookup retrieve every API page. Rules
  beyond the API's default page size remain in Terraform state, and a failed
  later page reports an error instead of removing a rule from state.
- The selected Go toolchain is now 1.26.9, and `golang.org/x/net` is now
  v0.60.0. These updates include security fixes.
- `zcp_instance` supports custom VM plans with `cpu`, `memory_gb`, and
  `disk_gb`. Fixed catalogue plans continue to use `plan`; custom inputs and
  `plan` are mutually exclusive.
- New `zcp_object_storage_key` creates and revokes object storage S3 access
  keys. Terraform stores the plaintext secret in state after create and
  preserves it on refresh after the API stops disclosing it.
- Object-storage key refresh keeps a disclosed secret in state after its
  visibility window closes, removes a revoked key from state, and preserves
  legacy object-storage credentials when a response does not disclose them.
- New `data.zcp_load_balancer` looks up a load balancer by slug and exposes its
  rule IDs for `zcp_load_balancer_attachment`.
- Load-balancer resources use the detail endpoint to refresh state and resolve
  rule IDs. During destroy, the provider treats an already absent load balancer
  as deleted.
- Load-balancer refresh clears `rule_id` when the detail response no longer
  contains the configured initial rule. Attachment destroy treats the exact 403
  response stating that the VM is invalid or not assigned to the rule as already
  detached while reporting other forbidden responses.
- Object storage reads support the current API storage-size response shape.
- Bucket configuration resources manage S3 gateway settings when the provider
  process has `ZCP_S3_ACCESS_KEY` and `ZCP_S3_SECRET_KEY` for an active store
  key.

## Live validation

On 2026-10-09, live Terraform validation covered ACL rule refresh across a rule
set expanded beyond the default page size, with zero changes on a second plan.
It also covered a custom 2 CPU, 2 GB memory, 20 GB disk instance; object-storage
key and bucket configuration create, refresh, and destroy; and load-balancer
creation, an additional rule, and the load-balancer data source.

The object-storage key test confirmed that the credential pair already in state
remained unchanged after the API disclosure window expired.

The load-balancer API omitted the configured initial rule on a fresh read. The
attachment create request succeeded, but a subsequent detach request reported
that the VM was not assigned to the rule. The validation did not confirm traffic
delivery or attachment membership. No backend cause has been established.

During cleanup, Terraform attachment cleanup completed after the API reported
that the VM was already unassigned, and the VM was destroyed. Terraform then
attempted to delete the remaining additional rule. The API rejected that request
because a load balancer must retain one active rule. The provider reports this
error and does not delete the load balancer when only rule destruction was
requested. The test load balancer and dedicated IP were removed through the CLI.
Terraform then removed the remaining network. Follow-up API checks found no test
resources or associated IPs, and all three Terraform states contain no managed
resources.
