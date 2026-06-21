package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// importPositional parses a slash-separated composite import ID and seeds the
// named attributes positionally into state. The first minRequired fields must be
// present and non-empty; any field beyond that may be omitted or left empty (its
// attribute stays null so the subsequent Read can populate it).
//
// This exists because several ZCP resources have write-only / create-only
// attributes (region, project, cloud_provider, plan, …) that the API never
// returns. A bare passthrough import would leave them null and produce a
// non-empty plan; carrying them in the import ID restores a zero-diff state.
func importPositional(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, fields []string, minRequired int, format string) {
	// SplitN with len(fields) so the LAST field captures any remaining slashes —
	// callers put free-text attributes (e.g. description) last so values
	// containing "/" survive a round-trip.
	parts := strings.SplitN(req.ID, "/", len(fields))
	if len(parts) < minRequired {
		resp.Diagnostics.AddError("Invalid import ID",
			fmt.Sprintf("expected format %q, got %q", format, req.ID))
		return
	}
	for i := 0; i < minRequired; i++ {
		if strings.TrimSpace(parts[i]) == "" {
			resp.Diagnostics.AddError("Invalid import ID",
				fmt.Sprintf("the first %d field(s) are required; expected format %q, got %q", minRequired, format, req.ID))
			return
		}
	}
	for i, f := range fields {
		if i >= len(parts) || strings.TrimSpace(parts[i]) == "" {
			continue
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(f), strings.TrimSpace(parts[i]))...)
	}
}
