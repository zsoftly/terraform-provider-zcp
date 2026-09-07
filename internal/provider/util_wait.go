package provider

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
)

// pollUntilGone calls exists every interval until it returns false (resource is confirmed gone)
// or the context is cancelled / deadline exceeded.
//
// The caller is responsible for setting a deadline on ctx (e.g. via context.WithTimeout
// derived from a resource's configured timeouts block).
//
// exists must return:
//   - (true,  nil) — resource still exists; keep polling
//   - (false, nil) — resource is confirmed gone; done
//   - (_,    err)  — recognized not-found errors are treated as gone; known
//     transient API errors are retried; all other errors stop the poll
//
// A transient list error does not abort the poll: many callers have already
// issued a delete/cancel request by the time they start polling, so aborting
// on the next transient error would report a destroy as failed when it
// actually succeeded. The last transient error is surfaced only if the
// resource does not resolve to gone before ctx's deadline.
func pollUntilGone(ctx context.Context, interval time.Duration, exists func(ctx context.Context) (bool, error)) error {
	var lastErr error
	for {
		found, err := exists(ctx)
		if err != nil {
			gone, transient := classifyPollUntilGoneError(err)
			if gone {
				return nil
			}
			if !transient {
				return err
			}
			lastErr = err
		} else {
			lastErr = nil
			if !found {
				return nil
			}
		}
		t := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !t.Stop() {
				<-t.C
			}
			if lastErr != nil {
				return lastErr
			}
			return ctx.Err()
		case <-t.C:
		}
	}
}

// classifyPollUntilGoneError identifies API responses that are safe to handle
// while waiting for deletion. Callers translate package-specific not-found
// sentinels to (false, nil) before invoking pollUntilGone.
func classifyPollUntilGoneError(err error) (gone, transient bool) {
	var apiErr *apierrors.APIError
	if !errors.As(err, &apiErr) {
		return false, false
	}
	if apierrors.IsNotFound(err) || apierrors.IsResourceNotFound(err) {
		return true, false
	}
	// The backend returns this documented message with HTTP 500 after a
	// successful delete. Do not apply the legacy text workaround to other
	// statuses or unstructured errors.
	if apiErr.StatusCode == http.StatusInternalServerError &&
		strings.Contains(strings.ToLower(apiErr.Message), "no query results for model") {
		return true, false
	}
	if apierrors.IsTransientRoutingError(err) {
		return false, true
	}

	switch {
	case apiErr.StatusCode == http.StatusRequestTimeout,
		apiErr.StatusCode == http.StatusTooManyRequests,
		apiErr.StatusCode >= http.StatusInternalServerError && apiErr.StatusCode < 600:
		return false, true
	default:
		return false, false
	}
}

// pollUntilReady polls check every interval until it reports the resource is
// ready (done == true) or the context is cancelled / deadline exceeded.
//
// check is invoked immediately on entry (before the first sleep) so a resource
// that is already in the target state returns without delay — this also keeps
// unit tests that supply an already-ready fake from blocking on the interval.
//
// check must return:
//   - (true,  nil) — resource reached the target state; done
//   - (false, nil) — not there yet; keep polling
//   - (_,    err)  — terminal error (e.g. a failure state); stop and surface it
func pollUntilReady(ctx context.Context, interval time.Duration, check func(ctx context.Context) (done bool, err error)) error {
	for {
		done, err := check(ctx)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		t := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !t.Stop() {
				<-t.C
			}
			return ctx.Err()
		case <-t.C:
		}
	}
}
