package discovery

import "testing"

func TestCameraLikeHTTPSignatures(t *testing.T) {
	for _, signature := range []string{
		"HTTP/1.1 200 OK\r\nServer: SHIP 2.0\r\n",
		"HTTP/1.1 411 Length Required\r\nServer: debut/1.30\r\n",
		"HTTP/1.1 200 OK\r\nServer: TP-Link\r\n",
	} {
		if !isCameraLikeHTTPSignature(signature) {
			t.Fatalf("expected camera-like signature: %q", signature)
		}
	}
}

func TestClassifyTapoCandidate(t *testing.T) {
	manufacturer, model := classifyDevice(false, false, "HTTP/1.1 200 OK\r\nServer: SHIP 2.0\r\n")
	if manufacturer != "TP-Link" || model != "Tapo Camera Candidate" {
		t.Fatalf("unexpected classification: %s %s", manufacturer, model)
	}
}
