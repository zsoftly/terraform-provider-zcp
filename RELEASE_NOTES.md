# terraform-provider-zcp v0.3.0 Release Notes

v0.3.0 updates the provider to the zcp-cli SDK v0.0.31.

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
