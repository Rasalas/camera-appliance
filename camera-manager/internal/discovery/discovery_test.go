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

func TestHTTPOnlyCandidateIsNotDiscoverableCamera(t *testing.T) {
	if isDiscoverableCamera(false, false) {
		t.Fatal("HTTP-only network devices should not be offered as cameras")
	}
	if !isDiscoverableCamera(true, false) || !isDiscoverableCamera(false, true) {
		t.Fatal("RTSP or ONVIF devices should be discoverable")
	}
}
