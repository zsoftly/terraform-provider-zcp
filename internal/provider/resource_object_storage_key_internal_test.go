package provider

import (
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/objectstorage"
)

func TestHasNewObjectStorageKeyCredentialsVisibilityBoundary(t *testing.T) {
	visibleUntil := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	key := &objectstorage.Key{
		APIKey:             "access",
		APISecret:          "secret",
		Status:             "active",
		SecretVisibleUntil: visibleUntil.Format(time.RFC3339),
	}
	for _, tc := range []struct {
		name string
		now  time.Time
		want bool
	}{
		{name: "before expiry", now: visibleUntil.Add(-time.Nanosecond), want: true},
		{name: "at expiry", now: visibleUntil, want: false},
		{name: "after expiry", now: visibleUntil.Add(time.Nanosecond), want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasNewObjectStorageKeyCredentials(key, tc.now); got != tc.want {
				t.Fatalf("hasNewObjectStorageKeyCredentials() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestApplyObjectStorageKeyStateUsesCapturedTime(t *testing.T) {
	visibleUntil := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	key := &objectstorage.Key{
		ID:                 "key-1",
		APIKey:             "access",
		APISecret:          "secret",
		Status:             "active",
		SecretVisibleUntil: visibleUntil.Format(time.RFC3339),
	}
	model := objectStorageKeyResourceModel{}
	applyObjectStorageKeyState(&model, key, false, visibleUntil.Add(-time.Nanosecond))
	if got := model.APISecret.ValueString(); got != "secret" {
		t.Fatalf("APISecret = %q, want captured secret", got)
	}
}

func TestApplyObjectStorageKeyStatePreservesSecretAfterExpiry(t *testing.T) {
	visibleUntil := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	model := objectStorageKeyResourceModel{APISecret: types.StringValue("saved-secret")}
	key := &objectstorage.Key{
		ID:                 "key-1",
		APIKey:             "access",
		APISecret:          "expired-secret",
		Status:             "active",
		SecretVisibleUntil: visibleUntil.Format(time.RFC3339),
	}
	applyObjectStorageKeyState(&model, key, true, visibleUntil)
	if got := model.APISecret.ValueString(); got != "saved-secret" {
		t.Fatalf("APISecret = %q, want preserved secret", got)
	}
}
