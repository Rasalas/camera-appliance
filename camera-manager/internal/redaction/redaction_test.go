package redaction

import (
	"strings"
	"testing"
)

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }

func TestURLRedactsPassword(t *testing.T) {
	got := URL("rtsp://user:secret@192.168.1.20:554/stream2")
	want := "rtsp://user:******@192.168.1.20:554/stream2"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestTextRedactsCredentialURL(t *testing.T) {
	got := Text("open rtsp://user:secret@192.168.1.20:554/stream2")
	if got != "open rtsp://user:******@192.168.1.20:554/stream2" {
		t.Fatalf("unexpected redaction: %q", got)
	}
}

func TestTextRedactsEmptyUsernameURL(t *testing.T) {
	got := Text("probe failed: rtsp://:secretpw@192.168.1.20:554/")
	want := "probe failed: rtsp://:******@192.168.1.20:554/"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestTextRedactsSecretQueryParams(t *testing.T) {
	got := Text("request to https://cam.local/api?token=abc123&x=1")
	if !contains(got, "token=******") || contains(got, "abc123") {
		t.Fatalf("token not redacted: %q", got)
	}
}

func TestTextKeepsPasswordlessURL(t *testing.T) {
	got := Text("rtsp://192.168.1.20:554/stream1")
	if got != "rtsp://192.168.1.20:554/stream1" {
		t.Fatalf("url without userinfo must stay intact: %q", got)
	}
}
