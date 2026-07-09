package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
)

// requiresReplaceUnlessAdopting forces replacement only when the attribute has
// a known prior value in state. Use it on write-only ForceNew attributes the
// API never returns: after an import their prior value is null, so the first
// apply adopts the configured value in place instead of replacing the
// resource. The resource's Update method must copy the plan into state for the
// adoption to persist.
func requiresReplaceUnlessAdopting() planmodifier.String {
	return stringplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = !req.StateValue.IsNull()
		},
		"Replaced when the prior value is known; adopted in place right after import.",
		"Replaced when the prior value is known; adopted in place right after import.",
	)
}
