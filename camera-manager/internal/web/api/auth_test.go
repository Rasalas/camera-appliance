package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
