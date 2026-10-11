# terraform-provider-zcp Release Notes

## v1.0.0

Provider v1.0.0 uses the zcp-cli v1.0.1 SDK and adds separate compute and
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
  remains for existing state only. Changing an existing cluster to split-plan
  configuration forces replacement.
- Review the upgrade plan before applying. Existing resources are preserved in
  state, but creating or replacing an instance or cluster requires the new
  named-plan fields.
- `everyOtherDay` is rejected before a scheduler request because the platform
  persists the interval and then reports an error. Use `hourly`, `dailyAt`,
  `weeklyOn`, or `monthlyOn`.
- One-off VM backup restores and run-now operations remain CLI commands because
  Terraform does not model them as durable state.
- Automatic Kubernetes scale-out is unverified. Terraform records accepted
  autoscaling bounds and does not promise an immediate worker increase or stable
  root-volume subscriptions.
- Cluster cancellation can leave a network that requires separate deletion.
- Instance name updates change the hostname. They can restart a running instance
  or start a stopped instance. The provider waits for the asynchronous change
  before later updates.
- `zcp_volume` accepts a named storage plan and capacity together.

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
