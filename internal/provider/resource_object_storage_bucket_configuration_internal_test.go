package provider

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
)

func TestBucketConfigurationWriteRetriesConcurrentModification(t *testing.T) {
	attempts := 0
	err := withBucketConfigurationWrite(context.Background(), "store", "bucket", func() error {
		attempts++
		if attempts < 3 {
			return errors.New("S3 ConcurrentModification")
		}
		return nil
	})
	if err != nil || attempts != 3 {
		t.Fatalf("err=%v attempts=%d, want nil/3", err, attempts)
	}
}

func TestIsConcurrentModification(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"typed", minio.ErrorResponse{Code: "ConcurrentModification", Message: "generic"}, true},
		{"wrapped typed", fmt.Errorf("wrapped: %w", minio.ErrorResponse{Code: "ConcurrentModification", Message: "generic"}), true},
		{"other typed", minio.ErrorResponse{Code: "AccessDenied"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isConcurrentModification(tc.err); got != tc.want {
				t.Fatalf("got %t, want %t", got, tc.want)
			}
		})
	}
}

func TestBucketConfigurationWriteSerializesSameBucket(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		_ = withBucketConfigurationWrite(context.Background(), "locked", "bucket", func() error { close(entered); <-release; return nil })
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	enteredSecond := false
	err := withBucketConfigurationWrite(ctx, "locked", "bucket", func() error { enteredSecond = true; return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second writer error = %v, want deadline exceeded", err)
	}
	if enteredSecond {
		t.Fatal("second writer entered while first held the lock")
	}
	close(release)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first writer did not finish")
	}
}

func TestBucketConfigurationWriteDoesNotRetryOtherErrors(t *testing.T) {
	attempts := 0
	err := withBucketConfigurationWrite(context.Background(), "store", "other", func() error { attempts++; return errors.New("access denied") })
	if err == nil || attempts != 1 {
		t.Fatalf("err=%v attempts=%d, want error/1", err, attempts)
	}
}

func TestBucketConfigurationWriteHonorsContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	err := withBucketConfigurationWrite(ctx, "store", "deadline", func() error { return errors.New("ConcurrentModification") })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v, want deadline exceeded", err)
	}
}
