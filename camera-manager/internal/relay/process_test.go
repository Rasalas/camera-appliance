package relay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// exitingRelayManager replaces ssh with a shell script and counts starts.
func exitingRelayManager(t *testing.T, script string) (*Manager, func() int) {
	t.Helper()
	a := newTestManager(t)
	a.Probe = func(context.Context, string, string) error {
		return errors.New("closed")
	}
	var mu sync.Mutex
	starts := 0
	a.newCommand = func([]string) (*exec.Cmd, error) {
		mu.Lock()
		starts++
		mu.Unlock()
		return exec.Command("sh", "-c", script), nil
	}
	seedRelayCamera(t, a)
	return a, func() int {
		mu.Lock()
		defer mu.Unlock()
		return starts
	}
}

func waitForRelayState(t *testing.T, a *Manager, want string) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		statuses, err := a.Statuses(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(statuses) == 1 && statuses[0].ProcessState == want {
			return statuses[0]
		}
		if time.Now().After(deadline) {
			t.Fatalf("relay never reached state %q, last %+v", want, statuses)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRelayStatusReportsExitedChildAsNotRunning(t *testing.T) {
	a, _ := exitingRelayManager(t, "exit 3")

	started, err := a.Start(context.Background(), "nas")
	if err != nil {
		t.Fatal(err)
	}
	if started.ProcessState != "running" {
		t.Fatalf("expected freshly started relay, got %+v", started)
	}

	status := waitForRelayState(t, a, "stale")
	if !strings.Contains(status.Message, "Exit-Code 3") {
		t.Fatalf("expected exit code in message, got %q", status.Message)
	}
}

func TestEnsureBacksOffAfterRelayExitsImmediately(t *testing.T) {
	ctx := context.Background()
	a, starts := exitingRelayManager(t, "echo 'Permission denied' >&2; exit 255")

	if _, err := a.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	waitForRelayState(t, a, "stale")

	statuses, err := a.Ensure(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if starts() != 1 {
		t.Fatalf("expected no restart during backoff, got %d starts", starts())
	}
	if len(statuses) != 1 || statuses[0].ProcessState != "backoff" || !strings.Contains(statuses[0].LastError, "Exit-Code 255") {
		t.Fatalf("expected backoff with exit error, got %+v", statuses)
	}
	settings, err := a.store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings[relayRuntimeKey("nas", "backoff_until")] == "" {
		t.Fatalf("expected persisted backoff, got %+v", settings)
	}

	// Once the backoff expires, the relay is started again.
	if err := a.store.PutSettings(ctx, map[string]string{
		relayRuntimeKey("nas", "backoff_until"): time.Now().Add(-time.Second).UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	statuses, err = a.Ensure(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if starts() != 2 || len(statuses) != 1 || !statuses[0].Started {
		t.Fatalf("expected restart after backoff, got %d starts and %+v", starts(), statuses)
	}
}

func TestEnsureRestartsLongRunningRelayAfterExit(t *testing.T) {
	ctx := context.Background()
	a, starts := exitingRelayManager(t, "exit 255")

	if _, err := a.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	waitForRelayState(t, a, "stale")
	a.childMu.Lock()
	a.children["nas"].startedAt = time.Now().Add(-time.Hour)
	a.childMu.Unlock()

	statuses, err := a.Ensure(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if starts() != 2 || len(statuses) != 1 || !statuses[0].Started {
		t.Fatalf("expected immediate restart, got %d starts and %+v", starts(), statuses)
	}
	settings, err := a.store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings[relayRuntimeKey("nas", "last_exited_at")] == "" {
		t.Fatalf("expected recorded exit, got %+v", settings)
	}
}

func TestProcessAliveTreatsZombieAsDead(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("zombie detection reads /proc")
	}
	cmd := exec.Command("sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Wait() })
	pid := cmd.Process.Pid
	deadline := time.Now().Add(5 * time.Second)
	for {
		stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			t.Fatal(err)
		}
		if statIsZombie(stat) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("child never became a zombie: %s", stat)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if processAlive(pid) {
		t.Fatal("zombie reported as alive")
	}
}

func TestStatIsZombieParsesState(t *testing.T) {
	cases := map[string]bool{
		"123 (ssh) S 1 123 123 0":      false,
		"123 (ssh) Z 1 123 123 0":      true,
		"123 (odd ) name) Z 1 123 0":   true,
		"123 (odd ) Z name) S 1 123 0": false,
		"garbage":                      false,
		"123 (ssh)":                    false,
	}
	for stat, want := range cases {
		if got := statIsZombie([]byte(stat)); got != want {
			t.Errorf("statIsZombie(%q) = %t, want %t", stat, got, want)
		}
	}
}

func TestRotateRelayLogKeepsOneOldFile(t *testing.T) {
	path := t.TempDir() + "/nas.log"
	large := bytes.Repeat([]byte("x"), relayLogMaxBytes+1)
	if err := os.WriteFile(path+".1", []byte("older"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, large, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := rotateRelayLog(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected oversized log to move away, got %v", err)
	}
	rotated, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rotated) != len(large) {
		t.Fatalf("expected previous log in .1, got %d bytes", len(rotated))
	}

	if err := os.WriteFile(path, []byte("small"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rotateRelayLog(path); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "small" {
		t.Fatalf("expected small log to stay, got %q", data)
	}
}

func TestStartRotatesOversizedRelayLog(t *testing.T) {
	a, _ := exitingRelayManager(t, "echo started")
	logPath := a.relayLogPath("nas")
	if err := os.MkdirAll(a.relayStateDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, bytes.Repeat([]byte("x"), relayLogMaxBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := a.Start(context.Background(), "nas"); err != nil {
		t.Fatal(err)
	}
	waitForRelayState(t, a, "stale")
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > relayLogMaxBytes {
		t.Fatalf("expected fresh log after start, got %d bytes", info.Size())
	}
	if _, err := os.Stat(logPath + ".1"); err != nil {
		t.Fatalf("expected rotated log: %v", err)
	}
}
