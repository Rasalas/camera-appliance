package discovery

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/fingerprint"
	"camera-appliance/camera-manager/internal/onvif"
)

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

func TestProbeHostEnrichesIdentityFromONVIF(t *testing.T) {
	onvifSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "UsernameToken") {
			w.Header().Set("Content-Type", `application/soap+xml; charset=utf-8`)
			_, _ = w.Write([]byte(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><s:Fault><s:Code><s:Value>s:Sender</s:Value></s:Code><s:Reason><s:Text>Sender not authorized</s:Text></s:Reason></s:Fault></s:Body></s:Envelope>`))
			return
		}
		_, _ = w.Write([]byte(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tt="http://www.onvif.org/ver10/schema"><s:Body><GetDeviceInformationResponse xmlns="http://www.onvif.org/ver10/device/wsdl"><tt:Manufacturer>TP-Link</tt:Manufacturer><tt:Model>Tapo C320WS</tt:Model><tt:SerialNumber>SERIAL-42</tt:SerialNumber><tt:HardwareId>000009</tt:HardwareId></GetDeviceInformationResponse></s:Body></s:Envelope>`))
	}))
	defer onvifSrv.Close()

	client := &onvif.Client{Timeout: time.Second}
	info, err := client.GetDeviceInformation(context.Background(), onvifSrv.URL, "admin", "pw")
	if err != nil {
		t.Fatal(err)
	}
	if info.SerialNumber != "SERIAL-42" || info.HardwareID != "000009" || info.Manufacturer != "TP-Link" || info.Model != "Tapo C320WS" {
		t.Fatalf("unexpected onvif info: %+v", info)
	}

	fp := fingerprint.Normalize(fingerprint.Fingerprint{
		Manufacturer: info.Manufacturer,
		Model:        info.Model,
		SerialNumber: onvifSerial(&info),
		HardwareID:   onvifHardwareID(&info),
	})
	id := fingerprint.DeviceID(fp)
	if id != fingerprint.DeviceID(fp) {
		t.Fatal("serial-based identity must be deterministic")
	}
	if !strings.HasPrefix(id, "device_") {
		t.Fatal("expected hashed device id")
	}
	// The serial tier must win over any MAC-based identity of the same device.
	if fingerprint.DeviceID(fp) == fingerprint.DeviceID(fingerprint.Fingerprint{MACAddress: "AA:BB:CC:DD:EE:FF"}) {
		t.Fatal("serial tier should take precedence over mac tier")
	}
}
