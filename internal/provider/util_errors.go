package provider

import (
	"strings"

	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
)

// isBackendNotFound reports whether an error means the entity no longer exists.
// Besides plain 404s, several backend modules (roles, DNS domains) return
// 500 "No query results for model [...]" after a successful delete (verified
// live 2026-07-05), so that response is treated as not-found too.
func isBackendNotFound(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "no query results for model")
}
