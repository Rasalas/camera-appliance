package cli

import (
	"context"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/state"
)

func TestHTTPServerBoundsSlowClientsWithoutWriteTimeout(t *testing.T) {
	server := newHTTPServer("127.0.0.1:0", http.NotFoundHandler())
	if server.ReadHeaderTimeout != 10*time.Second || server.IdleTimeout != 120*time.Second {
		t.Fatalf("unexpected timeouts: header %s idle %s", server.ReadHeaderTimeout, server.IdleTimeout)
	}
	// The go2rtc WebSocket proxy streams indefinitely.
	if server.WriteTimeout != 0 || server.ReadTimeout != 0 {
		t.Fatalf("read/write timeouts would cut long-lived streams: read %s write %s", server.ReadTimeout, server.WriteTimeout)
	}
}

func TestWaitTimeoutReportsCompletionAndGivesUp(t *testing.T) {
	var done sync.WaitGroup
	done.Go(func() {})
	if !waitTimeout(&done, time.Second) {
		t.Fatal("finished workers should be reported as done")
	}
	var stuck sync.WaitGroup
	release := make(chan struct{})
	defer close(release)
	stuck.Go(func() { <-release })
	start := time.Now()
	if waitTimeout(&stuck, 20*time.Millisecond) {
		t.Fatal("a stuck worker should not be reported as done")
	}
	if time.Since(start) > time.Second {
		t.Fatal("waitTimeout did not respect its bound")
	}
}

func TestPruneHistoryPeriodicallyRunsAtStartAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store, err := state.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		pruneHistoryPeriodically(ctx, store, time.Hour)
	}()
	cancel()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("prune loop did not stop after cancellation")
	}
}
