// Package onvif implements the small slice of the ONVIF Device Service needed
// to enrich discovered cameras: GetDeviceInformation over SOAP with optional
// WS-UsernameToken digest authentication.
package onvif

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	actionDeviceInformation = "http://www.onvif.org/ver10/device/wsdl/GetDeviceInformation"
	deviceServicePath       = "/onvif/device_service"
	xmlnsWSSE               = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"
	passwordDigestType      = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest"
	nonceEncodingType       = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd#Base64Binary"
)

// Info carries the stable identity attributes reported by the device.
type Info struct {
	Manufacturer string
	Model        string
	SerialNumber string
	HardwareID   string
	Firmware     string
}

type Client struct {
	HTTPClient *http.Client
	Timeout    time.Duration
}

// GetDeviceInformation queries the device service. It first tries an
// unauthenticated request and retries with WS-UsernameToken digest auth when
// the device answers with an authorization fault and credentials are given.
func (c *Client) GetDeviceInformation(ctx context.Context, baseURL, username, password string) (Info, error) {
	info := Info{}
	endpoint := strings.TrimRight(baseURL, "/") + deviceServicePath
	// First try anonymously; many devices answer GetDeviceInformation without
	// credentials. Only on an authorization fault do we sign the retry.
	body, err := c.request(ctx, endpoint, "", "")
	if err != nil {
		if !isAuthorizationFault(err) || username == "" || password == "" {
			return info, err
		}
		body, err = c.request(ctx, endpoint, username, password)
		if err != nil {
			return info, err
		}
	}
	parseDeviceInformation(body, &info)
	return info, nil
}

func (c *Client) request(ctx context.Context, endpoint, username, password string) ([]byte, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	envelope := buildEnvelope(endpoint, username, password)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(envelope))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `application/soap+xml; charset=utf-8; action="`+actionDeviceInformation+`"`)

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 && !bytes.Contains(data, []byte("Fault")) {
		return nil, fmt.Errorf("onvif: %s", resp.Status)
	}
	if faultErr := faultError(data); faultErr != nil {
		return nil, faultErr
	}
	return data, nil
}

func buildEnvelope(to, username, password string) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" ` +
		`xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing">`)
	b.WriteString(`<s:Header>`)
	b.WriteString(`<a:Action s:mustUnderstand="1">` + actionDeviceInformation + `</a:Action>`)
	b.WriteString(`<a:MessageID>urn:uuid:` + randomHex16() + `</a:MessageID>`)
	b.WriteString(`<a:To s:mustUnderstand="1">` + escapeXML(to) + `</a:To>`)
	if username != "" {
		writeUsernameToken(&b, username, password, time.Now().UTC())
	}
	b.WriteString(`</s:Header>`)
	b.WriteString(`<s:Body><GetDeviceInformation xmlns="http://www.onvif.org/ver10/device/wsdl"/></s:Body>`)
	b.WriteString(`</s:Envelope>`)
	return b.Bytes()
}

func writeUsernameToken(b *bytes.Buffer, username, password string, created time.Time) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return
	}
	createdText := created.Format("2006-01-02T15:04:05Z")
	digest := passwordDigest(nonce, createdText, password)
	b.WriteString(`<Security s:mustUnderstand="1" xmlns="` + xmlnsWSSE + `" ` +
		`xmlns:s="http://www.w3.org/2003/05/soap-envelope">`)
	b.WriteString(`<UsernameToken>`)
	b.WriteString(`<Username>` + escapeXML(username) + `</Username>`)
	b.WriteString(`<Password Type="` + passwordDigestType + `">` + digest + `</Password>`)
	b.WriteString(`<Nonce EncodingType="` + nonceEncodingType + `">` + base64.StdEncoding.EncodeToString(nonce) + `</Nonce>`)
	b.WriteString(`<Created xmlns="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-utility-1.0.xsd">` + createdText + `</Created>`)
	b.WriteString(`</UsernameToken>`)
	b.WriteString(`</Security>`)
}

// passwordDigest implements WS-UsernameToken PasswordDigest:
// base64(SHA1(rawNonce + Created + Password)).
func passwordDigest(nonce []byte, created, password string) string {
	h := sha1.New()
	h.Write(nonce)
	h.Write([]byte(created))
	h.Write([]byte(password))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// DigestForTest exposes the digest computation for unit tests.
func DigestForTest(nonce []byte, created, password string) string {
	return passwordDigest(nonce, created, password)
}

type faultInfo struct {
	code    string
	message string
}

func (f *faultInfo) Error() string {
	if f.message != "" {
		return "onvif fault: " + f.message
	}
	return "onvif fault"
}

// AuthorizationError reports a fault caused by missing or wrong credentials;
// callers may retry with authenticated requests.
type AuthorizationError struct{ Fault faultInfo }

func (e *AuthorizationError) Error() string { return e.Fault.Error() }

func isAuthorizationFault(err error) bool {
	_, ok := err.(*AuthorizationError)
	return ok
}

func faultError(body []byte) error {
	if !bytes.Contains(body, []byte(":Fault")) && !bytes.Contains(body, []byte("<Fault")) &&
		!bytes.Contains(body, []byte("Fault ")) {
		return nil
	}
	code, message := parseFault(body)
	fault := &faultInfo{code: code, message: message}
	switch {
	case strings.Contains(strings.ToLower(fault.code), "sender"),
		strings.Contains(strings.ToLower(fault.code), "notauthorized"),
		strings.Contains(strings.ToLower(fault.message), "not authorized"),
		strings.Contains(strings.ToLower(fault.message), "authentic"),
		strings.Contains(strings.ToLower(fault.message), "usernametoken"):
		return &AuthorizationError{Fault: *fault}
	default:
		if fault.message != "" {
			return fmt.Errorf("onvif fault: %s", fault.message)
		}
		return nil
	}
}

// parseFault extracts the fault code value and reason text from a SOAP fault.
func parseFault(body []byte) (code, message string) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var current string
	var reasonParts []string
	for {
		token, err := decoder.Token()
		if err != nil {
			return code, strings.Join(reasonParts, " ")
		}
		switch t := token.(type) {
		case xml.StartElement:
			current = t.Name.Local
		case xml.CharData:
			value := strings.TrimSpace(string(t))
			if value == "" || current == "" {
				continue
			}
			switch current {
			case "Value":
				if code == "" {
					code = value
				}
			case "Text", "Reason", "faultstring":
				reasonParts = append(reasonParts, value)
			}
		case xml.EndElement:
			current = ""
		}
	}
}

// parseDeviceInformation walks the SOAP body and fills the known fields.
// Namespace prefixes are ignored via local-name matching in the token stream,
// which keeps this dependency-free and tolerant across vendors.
func parseDeviceInformation(body []byte, info *Info) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var current string
	for {
		token, err := decoder.Token()
		if err != nil {
			return
		}
		switch t := token.(type) {
		case xml.StartElement:
			current = t.Name.Local
		case xml.CharData:
			value := strings.TrimSpace(string(t))
			if value == "" || current == "" {
				continue
			}
			switch current {
			case "Manufacturer":
				setIfEmpty(&info.Manufacturer, value)
			case "Model":
				setIfEmpty(&info.Model, value)
			case "SerialNumber":
				setIfEmpty(&info.SerialNumber, value)
			case "HardwareId":
				setIfEmpty(&info.HardwareID, value)
			case "FirmwareVersion":
				setIfEmpty(&info.Firmware, value)
			}
		case xml.EndElement:
			current = ""
		}
	}
}

func setIfEmpty(target *string, value string) {
	if *target == "" {
		*target = value
	}
}

func escapeXML(value string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(value)
}

func randomHex16() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-0000-0000-000000000000"
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
