package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/app"
	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/state"
)

func TestAdminAPIRequiresSessionAfterPasswordIsSet(t *testing.T) {
	ctx := context.Background()
	a := newAuthTestApp(t)
	if err := a.SetAuthPassword(ctx, "admin", "admin-pass"); err != nil {
		t.Fatal(err)
	}
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings[app.AuthSettingAdminPasswordHash] == "" || settings[app.AuthSettingAdminPasswordHash] == "admin-pass" {
		t.Fatalf("expected hashed admin password, got %q", settings[app.AuthSettingAdminPasswordHash])
	}
	handler := New(a).Handler()

	res := performJSON(handler, http.MethodGet, "/api/settings", nil, nil)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized settings response, got %d: %s", res.Code, res.Body.String())
	}

	cookie := loginCookie(t, handler, "admin", "admin-pass")
	res = performJSON(handler, http.MethodGet, "/api/settings", nil, cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("expected settings with admin session, got %d: %s", res.Code, res.Body.String())
	}
}

func TestViewerRoleCanOnlyReadViewerAPI(t *testing.T) {
	ctx := context.Background()
	a := newAuthTestApp(t)
	if err := a.SetAuthPassword(ctx, "admin", "admin-pass"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetAuthPassword(ctx, "viewer", "viewer-pass"); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.PutSettings(ctx, map[string]string{app.AuthSettingViewerPublic: "false"}); err != nil {
		t.Fatal(err)
	}
	handler := New(a).Handler()
	cookie := loginCookie(t, handler, "viewer", "viewer-pass")

	res := performJSON(handler, http.MethodGet, "/api/viewer", nil, cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("expected viewer API with viewer session, got %d: %s", res.Code, res.Body.String())
	}
	res = performJSON(handler, http.MethodGet, "/api/settings", nil, cookie)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden admin API for viewer, got %d: %s", res.Code, res.Body.String())
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	ctx := context.Background()
	a := newAuthTestApp(t)
	if err := a.SetAuthPassword(ctx, "admin", "admin-pass"); err != nil {
		t.Fatal(err)
	}
	handler := New(a).Handler()
	cookie := loginCookie(t, handler, "admin", "admin-pass")

	res := performJSON(handler, http.MethodPost, "/api/auth/logout", map[string]string{}, cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("expected logout success, got %d: %s", res.Code, res.Body.String())
	}
	res = performJSON(handler, http.MethodGet, "/api/settings", nil, cookie)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected session to be invalidated, got %d: %s", res.Code, res.Body.String())
	}
}

func TestGo2RTCAssetProxyServesAllowedPlayerModule(t *testing.T) {
	go2rtc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/video-stream.js" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/javascript")
		_, _ = w.Write([]byte("import {VideoRTC} from './video-rtc.js';\ncustomElements.define('video-stream', class extends HTMLElement {});\n"))
	}))
	defer go2rtc.Close()

	a := newAuthTestApp(t)
	a.Config.Go2RTCURL = go2rtc.URL
	handler := New(a).Handler()

	req := httptest.NewRequest(http.MethodGet, "/go2rtc/video-stream.js", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected proxied asset, got %d: %s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("expected javascript content type, got %q", got)
	}
	if body := res.Body.String(); len(body) < 6 || body[:6] != "import" {
		t.Fatalf("expected javascript body, got %q", body)
	}
}

func TestGo2RTCWebSocketProxyUsesGo2RTCOrigin(t *testing.T) {
	var seenPath, seenQuery, seenOrigin, seenHost string
	go2rtc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenQuery = r.URL.RawQuery
		seenOrigin = r.Header.Get("Origin")
		seenHost = r.Host
		writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
	}))
	defer go2rtc.Close()

	a := newAuthTestApp(t)
	a.Config.Go2RTCURL = go2rtc.URL
	handler := New(a).Handler()

	req := httptest.NewRequest(http.MethodGet, "/go2rtc/api/ws?src=cam1", nil)
	req.Header.Set("Origin", "http://"+req.Host)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected proxied websocket endpoint, got %d: %s", res.Code, res.Body.String())
	}
	if seenPath != "/api/ws" || seenQuery != "src=cam1" {
		t.Fatalf("unexpected proxied target path/query: %s?%s", seenPath, seenQuery)
	}
	if seenOrigin != go2rtc.URL || seenHost == "" {
		t.Fatalf("expected go2rtc origin and host, got origin=%q host=%q", seenOrigin, seenHost)
	}
}

func TestGo2RTCWebSocketProxyRejectsCrossOrigin(t *testing.T) {
	go2rtc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request should not be proxied, got %s", r.URL.Path)
	}))
	defer go2rtc.Close()

	a := newAuthTestApp(t)
	a.Config.Go2RTCURL = go2rtc.URL
	handler := New(a).Handler()

	for _, origin := range []string{"", "http://evil.example", "https://127.0.0.1:9999"} {
		req := httptest.NewRequest(http.MethodGet, "/go2rtc/api/ws?src=cam1", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusForbidden {
			t.Fatalf("origin %q: expected 403, got %d", origin, res.Code)
		}
	}
}

func TestGo2RTCWebSocketRequiresViewerRoleWhenAuthEnabled(t *testing.T) {
	go2rtc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
	}))
	defer go2rtc.Close()

	a := newAuthTestApp(t)
	a.Config.Go2RTCURL = go2rtc.URL
	if err := a.Store.PutSettings(context.Background(), map[string]string{
		app.AuthSettingAdminPasswordHash: "x",
		app.AuthSettingViewerPublic:      "false",
	}); err != nil {
		t.Fatal(err)
	}
	handler := New(a).Handler()

	req := httptest.NewRequest(http.MethodGet, "/go2rtc/api/ws?src=cam1", nil)
	req.Header.Set("Origin", "http://"+req.Host)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session, got %d", res.Code)
	}

	publicReq := httptest.NewRequest(http.MethodGet, "/go2rtc/api/ws?src=cam1", nil)
	publicReq.Header.Set("Origin", "http://"+publicReq.Host)
	publicRes := httptest.NewRecorder()
	if err := a.Store.PutSettings(context.Background(), map[string]string{app.AuthSettingViewerPublic: "true"}); err != nil {
		t.Fatal(err)
	}
	handler.ServeHTTP(publicRes, publicReq)
	if publicRes.Code != http.StatusOK {
		t.Fatalf("expected viewer-public websocket to pass, got %d", publicRes.Code)
	}
}

func TestLoginRateLimitBlocksAfterRepeatedFailures(t *testing.T) {
	a := newAuthTestApp(t)
	if err := a.SetAuthPassword(context.Background(), "admin", "secret-password"); err != nil {
		t.Fatal(err)
	}
	handler := New(a).Handler()
	body := strings.NewReader(`{"username":"admin","password":"wrong"}`)

	var status int
	for i := 0; i < 15; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
		body.Reset(`{"username":"admin","password":"wrong"}`)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		status = res.Code
		if status == http.StatusTooManyRequests {
			return
		}
	}
	t.Fatalf("expected rate limit to kick in, last status %d", status)
}

func TestLocalAdminBypassRejectsForeignHostHeader(t *testing.T) {
	a := newAuthTestApp(t)
	if err := a.Store.PutSettings(context.Background(), map[string]string{
		app.AuthSettingLocalAdminBypass:  "true",
		app.AuthSettingAdminPasswordHash: "x",
	}); err != nil {
		t.Fatal(err)
	}
	handler := New(a).Handler()

	loopbackReq := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	loopbackReq.RemoteAddr = "127.0.0.1:40000"
	loopbackReq.Host = "127.0.0.1:8091"
	loopbackRes := httptest.NewRecorder()
	handler.ServeHTTP(loopbackRes, loopbackReq)

	rebindReq := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rebindReq.RemoteAddr = "127.0.0.1:40001"
	rebindReq.Host = "evil.example:8091"
	rebindRes := httptest.NewRecorder()
	handler.ServeHTTP(rebindRes, rebindReq)

	if loopbackRes.Code != http.StatusOK {
		t.Fatalf("loopback host should keep bypass, got %d", loopbackRes.Code)
	}
	if rebindRes.Code != http.StatusUnauthorized {
		t.Fatalf("foreign host must not get bypass, got %d", rebindRes.Code)
	}
}

func TestCrossOriginStateChangingRequestRejected(t *testing.T) {
	a := newAuthTestApp(t)
	handler := New(a).Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/discovery", nil)
	req.Header.Set("Origin", "http://evil.example")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-origin POST, got %d", res.Code)
	}
}

func TestDeviceFrameUsesConfiguredRTSPEndpoint(t *testing.T) {
	ctx := context.Background()
	a := newAuthTestApp(t)
	a.RTSPProbe = func(_ context.Context, host, port string) error {
		if host == "192.168.1.20" && port == "554" {
			return context.DeadlineExceeded
		}
		return nil
	}
	if err := a.Store.PutSettings(ctx, map[string]string{
		"camera.rtsp_endpoint.dev1.host": "host.docker.internal",
		"camera.rtsp_endpoint.dev1.port": "15541",
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertDevice(ctx, state.Device{ID: "dev1", LastIP: "192.168.1.20"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertBinding(ctx, state.Binding{SlotID: "cam1", DeviceID: "dev1", Username: "user", StreamName: "stream2", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	var capturedURL string
	server := New(a)
	server.cameras.Capture = func(_ context.Context, rawURL, _ string) ([]byte, error) {
		capturedURL = rawURL
		return []byte("jpeg"), nil
	}

	res := performJSON(server.Handler(), http.MethodPost, "/api/devices/dev1/frame", map[string]any{
		"username": "user",
		"password": "secret",
		"stream":   "stream2",
	}, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("expected frame capture success, got %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(capturedURL, "@host.docker.internal:15541/stream2") {
		t.Fatalf("expected capture through configured RTSP endpoint, got %q", capturedURL)
	}
}

func TestDeviceProbeUsesConfiguredRTSPEndpoint(t *testing.T) {
	ctx := context.Background()
	a := newAuthTestApp(t)
	a.RTSPProbe = func(_ context.Context, host, port string) error {
		if host == "192.168.1.20" && port == "554" {
			return context.DeadlineExceeded
		}
		return nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.PutSettings(ctx, map[string]string{
		"camera.rtsp_endpoint.dev1.host": "host.docker.internal",
		"camera.rtsp_endpoint.dev1.port": port,
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertDevice(ctx, state.Device{ID: "dev1", LastIP: "192.168.1.20"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertBinding(ctx, state.Binding{SlotID: "cam1", DeviceID: "dev1", Username: "user", StreamName: "stream2", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	res := performJSON(New(a).Handler(), http.MethodPost, "/api/devices/dev1/probe", map[string]any{
		"username": "user",
		"password": "secret",
		"stream":   "stream2",
	}, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("expected probe success response, got %d: %s", res.Code, res.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["success"] != true {
		t.Fatalf("expected probe success through configured endpoint, got %+v", body)
	}
	if !strings.Contains(body["url_redacted"].(string), "@host.docker.internal:"+port+"/stream2") {
		t.Fatalf("expected redacted URL to use configured endpoint, got %+v", body)
	}
	<-done
}

func newAuthTestApp(t *testing.T) *app.App {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	store, err := state.Open(ctx, filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	slots := config.DefaultSlots()
	if err := store.UpsertSlots(ctx, slots); err != nil {
		t.Fatal(err)
	}
	return &app.App{
		Config: config.Config{
			ConfigDir:      dir,
			StateDir:       dir,
			Go2RTCURL:      "http://127.0.0.1:1",
			Go2RTCRTSPURL:  "rtsp://127.0.0.1:8554",
			FrontendDist:   dir,
			RequestTimeout: 50 * time.Millisecond,
			CaptureSSHHost: "",
		},
		Store: store,
		Slots: slots,
		RTSPProbe: func(context.Context, string, string) error {
			return nil
		},
	}
}

func loginCookie(t *testing.T, handler http.Handler, username, password string) *http.Cookie {
	t.Helper()
	res := performJSON(handler, http.MethodPost, "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	}, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("expected login success, got %d: %s", res.Code, res.Body.String())
	}
	for _, cookie := range res.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			return cookie
		}
	}
	t.Fatal("login did not set session cookie")
	return nil
}

func performJSON(handler http.Handler, method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func TestLANAccessRequiresAdminPassword(t *testing.T) {
	a := newAuthTestApp(t)
	res := performJSON(New(a).Handler(), http.MethodPut, "/api/settings", map[string]string{app.NetworkSettingLANAccess: "true"}, nil)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected LAN access without admin password to be rejected, got %d: %s", res.Code, res.Body.String())
	}
}

func TestLANAccessCanBeEnabledAfterAdminPasswordIsSet(t *testing.T) {
	ctx := context.Background()
	a := newAuthTestApp(t)
	if err := a.SetAuthPassword(ctx, "admin", "admin-pass"); err != nil {
		t.Fatal(err)
	}
	handler := New(a).Handler()
	cookie := loginCookie(t, handler, "admin", "admin-pass")
	res := performJSON(handler, http.MethodPut, "/api/settings", map[string]string{app.NetworkSettingLANAccess: "true"}, cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("expected LAN access setting to be accepted, got %d: %s", res.Code, res.Body.String())
	}
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings[app.NetworkSettingLANAccess] != "true" {
		t.Fatalf("expected persisted LAN setting, got %q", settings[app.NetworkSettingLANAccess])
	}
}

func TestRememberedLoginUsesLongLivedCookie(t *testing.T) {
	ctx := context.Background()
	a := newAuthTestApp(t)
	if err := a.SetAuthPassword(ctx, "admin", "admin-pass"); err != nil {
		t.Fatal(err)
	}
	res := performJSON(New(a).Handler(), http.MethodPost, "/api/auth/login", map[string]any{"username": "admin", "password": "admin-pass", "remember": true}, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("expected login success, got %d: %s", res.Code, res.Body.String())
	}
	cookies := res.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one session cookie, got %d", len(cookies))
	}
	if cookies[0].MaxAge < 29*24*60*60 {
		t.Fatalf("expected remembered login for about 30 days, got MaxAge=%d", cookies[0].MaxAge)
	}
}

func TestLANAccessRejectsNonCanonicalBoolean(t *testing.T) {
	a := newAuthTestApp(t)
	if err := a.SetAuthPassword(context.Background(), "admin", "admin-pass"); err != nil {
		t.Fatal(err)
	}
	handler := New(a).Handler()
	cookie := loginCookie(t, handler, "admin", "admin-pass")
	for _, value := range []string{"1", "yes", "on"} {
		res := performJSON(handler, http.MethodPut, "/api/settings", map[string]string{app.NetworkSettingLANAccess: value}, cookie)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("expected %q to be rejected, got %d: %s", value, res.Code, res.Body.String())
		}
	}
}

func TestNormalLoginUsesBrowserSessionCookie(t *testing.T) {
	a := newAuthTestApp(t)
	if err := a.SetAuthPassword(context.Background(), "admin", "admin-pass"); err != nil {
		t.Fatal(err)
	}
	res := performJSON(New(a).Handler(), http.MethodPost, "/api/auth/login", map[string]any{"username": "admin", "password": "admin-pass", "remember": false}, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("expected login success, got %d: %s", res.Code, res.Body.String())
	}
	cookies := res.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one session cookie, got %d", len(cookies))
	}
	if cookies[0].MaxAge != 0 || !cookies[0].Expires.IsZero() {
		t.Fatalf("expected browser-session cookie, got MaxAge=%d Expires=%v", cookies[0].MaxAge, cookies[0].Expires)
	}
}
