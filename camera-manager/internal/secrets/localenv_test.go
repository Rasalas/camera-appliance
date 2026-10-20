package secrets

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestWriteLocalEnvReplacesFileInsteadOfTruncating(t *testing.T) {
	dir := t.TempDir()
	if err := writeLocalEnvKey(dir, "CAMERA_PASSWORD_A", "first"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "local.env")
	old, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	before, _ := old.Stat()

	if err := writeLocalEnvKey(dir, "CAMERA_PASSWORD_B", "second"); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatal("local.env was rewritten in place; a crash mid-write would lose every password")
	}
	// A reader holding the old file still sees complete old content.
	data, err := io.ReadAll(old)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "CAMERA_PASSWORD_A='first'\n" {
		t.Fatalf("old file changed: %q", data)
	}
	if after.Mode().Perm() != 0o600 {
		t.Fatalf("local.env mode %o, want 600", after.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
}

func TestConcurrentLocalEnvUpdatesKeepEveryKey(t *testing.T) {
	dir := t.TempDir()
	if err := writeLocalEnvKey(dir, "DROP", "x"); err != nil {
		t.Fatal(err)
	}
	const writers = 40
	var wg sync.WaitGroup
	for i := range writers {
		wg.Go(func() {
			if err := writeLocalEnvKey(dir, fmt.Sprintf("CAMERA_PASSWORD_%d", i), fmt.Sprintf("pw-%d", i)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Go(func() { removeLocalEnvKey(dir, "DROP") })
	wg.Wait()
	for i := range writers {
		if got := readLocalEnvKey(dir, fmt.Sprintf("CAMERA_PASSWORD_%d", i)); got != fmt.Sprintf("pw-%d", i) {
			t.Fatalf("CAMERA_PASSWORD_%d lost in concurrent save: %q", i, got)
		}
	}
	if readLocalEnvKey(dir, "DROP") != "" {
		t.Fatal("concurrent remove was lost")
	}
}

func TestRemoveLocalEnvKeyDoesNotCreateFile(t *testing.T) {
	dir := t.TempDir()
	removeLocalEnvKey(dir, "MISSING")
	if _, err := os.Stat(filepath.Join(dir, "local.env")); !os.IsNotExist(err) {
		t.Fatalf("remove must not create local.env: %v", err)
	}
}

func TestSecretToolIsKilledAfterTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	script := filepath.Join(t.TempDir(), "secret-tool")
	// The child sleep keeps stdout open after the shell is killed.
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	previous := secretToolCommand
	secretToolCommand = script
	t.Cleanup(func() { secretToolCommand = previous })

	start := time.Now()
	if _, err := runSecretTool(100*time.Millisecond, nil, "lookup"); err == nil {
		t.Fatal("expected a timeout error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("secret-tool call blocked for %s", elapsed)
	}
}
