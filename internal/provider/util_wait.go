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
//   - (_,    err)  — real API error; stop and surface it
func pollUntilGone(ctx context.Context, interval time.Duration, exists func(ctx context.Context) (bool, error)) error {
	for {
		found, err := exists(ctx)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
