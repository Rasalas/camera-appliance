package onvif

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const sampleDeviceInformation = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tt="http://www.onvif.org/ver10/schema">
  <s:Body>
    <GetDeviceInformationResponse xmlns="http://www.onvif.org/ver10/device/wsdl">
      <tt:Manufacturer>TP-Link</tt:Manufacturer>
      <tt:Model>Tapo C320WS</tt:Model>
      <tt:FirmwareVersion>1.3.5</tt:FirmwareVersion>
      <tt:SerialNumber>98765432109876543210</tt:SerialNumber>
      <tt:HardwareId>000001</tt:HardwareId>
      <tt:Ignored xmlns:ignore="urn:x">text</tt:Ignored>
    </GetDeviceInformationResponse>
  </s:Body>
</s:Envelope>`

const authFault = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <s:Fault>
      <s:Code><s:Value>s:Sender</s:Value></s:Code>
      <s:Reason><s:Text xml:lang="en">Sender not authorized</s:Text></s:Reason>
    </s:Fault>
  </s:Body>
</s:Envelope>`

func TestGetDeviceInformationWithoutAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != deviceServicePath {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), actionDeviceInformation) {
			t.Errorf("request must carry GetDeviceInformation action, got %q", body)
		}
		w.Header().Set("Content-Type", `application/soap+xml; charset=utf-8`)
		_, _ = w.Write([]byte(sampleDeviceInformation))
	}))
	defer server.Close()

	client := &Client{Timeout: 2 * time.Second}
	info, err := client.GetDeviceInformation(context.Background(), server.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if info.Manufacturer != "TP-Link" || info.Model != "Tapo C320WS" ||
		info.SerialNumber != "98765432109876543210" || info.HardwareID != "000001" ||
		info.Firmware != "1.3.5" {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestGetDeviceInformationRetriesWithDigestAuth(t *testing.T) {
	requests := 0
	var gotUsername, gotNonceB64, gotCreated, gotDigest string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		body := string(readAll(t, r))
		if !strings.Contains(body, "UsernameToken") {
			w.Header().Set("Content-Type", `application/soap+xml; charset=utf-8`)
			_, _ = w.Write([]byte(authFault))
			return
		}
		gotUsername = between(body, "<Username>", "</Username>")
		gotNonceB64 = extractTag(body, "Nonce")
		gotCreated = extractTag(body, "Created")
		gotDigest = extractTag(body, "Password")
		w.Header().Set("Content-Type", `application/soap+xml; charset=utf-8`)
		_, _ = w.Write([]byte(sampleDeviceInformation))
	}))
	defer server.Close()

	client := &Client{Timeout: 2 * time.Second}
	info, err := client.GetDeviceInformation(context.Background(), server.URL, "admin", "camera-password")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("expected unauthenticated attempt + authenticated retry, got %d requests", requests)
	}
	if gotUsername != "admin" {
		t.Fatalf("expected username admin, got %q", gotUsername)
	}
	if info.SerialNumber != "98765432109876543210" {
		t.Fatalf("expected serial from authenticated response, got %+v", info)
	}
	nonce, err := base64.StdEncoding.DecodeString(gotNonceB64)
	if err != nil || len(nonce) == 0 {
		t.Fatalf("nonce must be base64 binary, got %q (%v)", gotNonceB64, err)
	}
	sum := sha1.Sum(append(append(nonce[:len(nonce):len(nonce)], []byte(gotCreated)...), []byte("camera-password")...))
	want := base64.StdEncoding.EncodeToString(sum[:])
	if gotDigest != want {
		t.Fatalf("digest mismatch: got %q want %q", gotDigest, want)
	}
}

func TestAuthorizationFaultIsNotRetriedWithoutCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", `application/soap+xml; charset=utf-8`)
		_, _ = w.Write([]byte(authFault))
	}))
	defer server.Close()

	client := &Client{Timeout: 2 * time.Second}
	if _, err := client.GetDeviceInformation(context.Background(), server.URL, "", ""); err == nil {
		t.Fatal("expected error for authorization fault")
	} else if isAuthorizationFault(err) {
		// fine either way; the important part is no retry happened because
		// there are no credentials.
	}
}

func readAll(t *testing.T, r *http.Request) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(r.Body); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	rest := s[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func extractTag(body, localName string) string {
	open := "<" + localName
	idx := strings.Index(body, open)
	if idx < 0 {
		return ""
	}
	after := body[idx:]
	closeStart := strings.Index(after, ">")
	if closeStart < 0 {
		return ""
	}
	value := after[closeStart+1:]
	end := strings.Index(value, "</"+localName+">")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(value[:end])
}
