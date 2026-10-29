package state

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestRestoreLockCoordinatesAccessPolicyEdits(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	unlock, err := LockAccessPolicy(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(context.Context) error{
		func(ctx context.Context) error {
			return store.PutSettings(ctx, map[string]string{"network.lan_access_enabled": "true"})
		},
		func(ctx context.Context) error {
			return store.ReplaceAuthPassword(ctx, "auth.admin.password_hash", "admin", "new-hash")
		},
	} {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
		err := edit(ctx)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			unlock()
			t.Fatalf("access policy edit ignored restore lock: %v", err)
		}
	}
	unlock()
	if err := store.PutSettings(ctx, map[string]string{"network.lan_access_enabled": "false"}); err != nil {
		t.Fatal(err)
	}
	settings, err := store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings["auth.admin.password_hash"] != "" {
		t.Fatal("blocked password change was published")
	}
}
