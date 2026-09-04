package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestConcurrentPasswordChangePreventsStaleLogin(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := "auth.admin.password_hash"
	if err := store.ReplaceAuthPassword(ctx, key, "admin", "old-hash"); err != nil {
		t.Fatal(err)
	}
	verifiedHash := "old-hash"
	if err := store.ReplaceAuthPassword(ctx, key, "admin", "new-hash"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAuthSessionForPassword(ctx, key, verifiedHash, AuthSession{TokenHash: "token", Role: "admin", ExpiresAt: time.Now().Add(time.Hour)}); err == nil {
		t.Fatal("login verified before password change created a session")
	}
	if _, err := store.AuthSession(ctx, "token", time.Now()); err == nil {
		t.Fatal("stale session persisted")
	}
}
