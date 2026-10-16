package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
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

// requiresReplaceBoolUnlessAdopting lets the first apply save an imported value
// without replacing the resource. Later changes to a stored value require
// replacement. The resource's Update method must copy the plan into state.
func requiresReplaceBoolUnlessAdopting() planmodifier.Bool {
	return boolplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.BoolRequest, resp *boolplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = !req.StateValue.IsNull()
		},
		"Replaced when the prior value is known; adopted in place right after import.",
		"Replaced when the prior value is known; adopted in place right after import.",
	)
}

// requiresReplaceIfConfigured only replaces a computed optional value when the
// configuration explicitly supplies a different known value.
func requiresReplaceIfConfigured() planmodifier.String {
	return stringplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = !req.ConfigValue.IsNull() && !req.ConfigValue.IsUnknown() && !req.StateValue.IsNull() && req.ConfigValue.ValueString() != req.StateValue.ValueString()
		},
		"Replaced when configured with a different value.",
		"Replaced when configured with a different value.",
	)
}
