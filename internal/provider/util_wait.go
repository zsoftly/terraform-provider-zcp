package provider

import (
	"context"
	"time"
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
//   - (_,    err)  — a list (or similar) call failed; treated as transient
//
// A single failed call does not abort the poll: many callers have already
// issued a delete/cancel request by the time they start polling, so aborting
// on the next transient list error would report a destroy as failed when it
// actually succeeded. Instead, an error keeps the poll going until ctx's
// deadline, and only the last error is surfaced if the resource never
// resolves to gone.
func pollUntilGone(ctx context.Context, interval time.Duration, exists func(ctx context.Context) (bool, error)) error {
	var lastErr error
	for {
		found, err := exists(ctx)
		if err != nil {
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
