package system

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/config"
)

func TestGo2RTCStatusReportsHTTPHealth(t *testing.T) {
	var healthy atomic.Bool
	healthy.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !healthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	cfg := config.Config{Go2RTCURL: server.URL}

	if status := Go2RTCStatus(context.Background(), cfg); !status.Online || status.Name != "go2rtc" {
		t.Fatalf("expected online go2rtc, got %+v", status)
	}
	healthy.Store(false)
	if status := Go2RTCStatus(context.Background(), cfg); status.Online {
		t.Fatalf("expected offline go2rtc, got %+v", status)
	}
}

func TestGo2RTCStatusRedactsMalformedURL(t *testing.T) {
	status := Go2RTCStatus(context.Background(), config.Config{Go2RTCURL: "http://admin:fixture-secret@[broken"})
	if status.Online {
		t.Fatalf("expected malformed URL to be offline, got %+v", status)
	}
	if strings.Contains(status.Message, "fixture-secret") {
		t.Fatalf("status message leaked credentials: %q", status.Message)
	}
}

func TestCommandOutputReturnsWhenChildHoldsPipeAfterTimeout(t *testing.T) {
	setCommandWaitDelay(t, 200*time.Millisecond)
	started := time.Now()
	_, err := commandOutput(context.Background(), 100*time.Millisecond, "sh", "-c", "sleep 30 & sleep 30")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("command blocked for %s after timeout", elapsed)
	}
}

func TestRestartGo2RTCSucceedsWhenCommandLeavesBackgroundChild(t *testing.T) {
	setCommandWaitDelay(t, 200*time.Millisecond)
	script := filepath.Join(t.TempDir(), "restart-go2rtc")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30 &\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := RestartGo2RTC(context.Background(), config.Config{Go2RTCRestart: script}); err != nil {
		t.Fatalf("expected successful restart, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("restart blocked for %s on inherited output pipe", elapsed)
	}
}

func setCommandWaitDelay(t *testing.T, delay time.Duration) {
	t.Helper()
	previous := commandWaitDelay
	commandWaitDelay = delay
	t.Cleanup(func() { commandWaitDelay = previous })
}
