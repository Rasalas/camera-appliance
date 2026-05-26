package redaction

import "testing"

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
