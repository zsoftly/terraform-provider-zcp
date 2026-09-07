package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
)

func pollUntilGoneTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestPollUntilGoneTreatsAPINotFoundAsGone(t *testing.T) {
	calls := 0
	err := pollUntilGone(pollUntilGoneTestContext(t), time.Millisecond, func(context.Context) (bool, error) {
		calls++
		return false, &apierrors.APIError{StatusCode: 404, Message: "not found"}
	})
	if err != nil {
		t.Fatalf("pollUntilGone() error = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("pollUntilGone() calls = %d, want 1", calls)
	}
}

func TestPollUntilGoneTreatsRecognizedAPIResourceNotFoundAsGone(t *testing.T) {
	calls := 0
	err := pollUntilGone(pollUntilGoneTestContext(t), time.Millisecond, func(context.Context) (bool, error) {
		calls++
		return false, &apierrors.APIError{StatusCode: 403, Message: "The selected service not found."}
	})
	if err != nil {
		t.Fatalf("pollUntilGone() error = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("pollUntilGone() calls = %d, want 1", calls)
	}
}

func TestPollUntilGoneTreatsLegacyBackendNotFoundAsGone(t *testing.T) {
	calls := 0
	err := pollUntilGone(pollUntilGoneTestContext(t), time.Millisecond, func(context.Context) (bool, error) {
		calls++
		return false, &apierrors.APIError{StatusCode: 500, Message: "No query results for model [Domain]"}
	})
	if err != nil {
		t.Fatalf("pollUntilGone() error = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("pollUntilGone() calls = %d, want 1", calls)
	}
}

func TestPollUntilGoneRetriesTransientServerError(t *testing.T) {
	calls := 0
	err := pollUntilGone(pollUntilGoneTestContext(t), time.Millisecond, func(context.Context) (bool, error) {
		calls++
		if calls == 1 {
			return false, &apierrors.APIError{StatusCode: 500, Message: "internal error"}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("pollUntilGone() error = %v, want nil", err)
	}
	if calls != 2 {
		t.Errorf("pollUntilGone() calls = %d, want 2", calls)
	}
}

func TestPollUntilGoneReturnsPermanentAPIErrorsImmediately(t *testing.T) {
	for _, tc := range []struct {
		name       string
		statusCode int
	}{
		{name: "unauthorized", statusCode: 401},
		{name: "forbidden", statusCode: 403},
		{name: "unprocessable entity", statusCode: 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			want := &apierrors.APIError{StatusCode: tc.statusCode, Message: "request rejected"}
			err := pollUntilGone(pollUntilGoneTestContext(t), time.Hour, func(context.Context) (bool, error) {
				calls++
				return false, want
			})
			if !errors.Is(err, want) {
				t.Fatalf("pollUntilGone() error = %v, want %v", err, want)
			}
			if calls != 1 {
				t.Errorf("pollUntilGone() calls = %d, want 1", calls)
			}
		})
	}
}

func TestPollUntilGoneReturnsUnrecognizedErrorImmediately(t *testing.T) {
	calls := 0
	want := errors.New("unexpected failure")
	err := pollUntilGone(pollUntilGoneTestContext(t), time.Hour, func(context.Context) (bool, error) {
		calls++
		return false, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("pollUntilGone() error = %v, want %v", err, want)
	}
	if calls != 1 {
		t.Errorf("pollUntilGone() calls = %d, want 1", calls)
	}
}

func TestPollUntilGoneDoesNotTreatMarkerCollisionsAsGone(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{
			name: "unauthorized API error",
			err:  &apierrors.APIError{StatusCode: 401, Message: "No query results for model"},
		},
		{
			name: "unprocessable entity API error",
			err:  &apierrors.APIError{StatusCode: 422, Message: "No query results for model"},
		},
		{
			name: "unstructured error",
			err:  errors.New("No query results for model"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			err := pollUntilGone(pollUntilGoneTestContext(t), time.Hour, func(context.Context) (bool, error) {
				calls++
				return false, tc.err
			})
			if !errors.Is(err, tc.err) {
				t.Fatalf("pollUntilGone() error = %v, want %v", err, tc.err)
			}
			if calls != 1 {
				t.Errorf("pollUntilGone() calls = %d, want 1", calls)
			}
		})
	}
}

func TestPollUntilGoneReturnsLastTransientErrorAtDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	want := &apierrors.APIError{StatusCode: 500, Message: "internal error"}
	err := pollUntilGone(ctx, time.Millisecond, func(context.Context) (bool, error) {
		return false, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("pollUntilGone() error = %v, want %v", err, want)
	}
}
