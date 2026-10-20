package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestStaticCacheHeaders(t *testing.T) {
	a := newAuthTestApp(t)
	dist := t.TempDir()
	a.Config.FrontendDist = dist
	if err := os.MkdirAll(filepath.Join(dist, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"index.html":           "<html></html>",
		"assets/index-AbC1.js": "console.log(1)",
		"favicon.svg":          "<svg/>",
	} {
		if err := os.WriteFile(filepath.Join(dist, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(a).Handler()
	cases := []struct {
		path, want string
	}{
		{"/assets/index-AbC1.js", "public, max-age=31536000, immutable"},
		{"/", "no-cache"},
		{"/kamera/cam1", "no-cache"},
		{"/assets/missing-XyZ.js", "no-cache"},
		{"/favicon.svg", "no-cache"},
	}
	for _, tc := range cases {
		res := performJSON(handler, http.MethodGet, tc.path, nil, nil)
		if res.Code != http.StatusOK {
			t.Fatalf("%s: status %d", tc.path, res.Code)
		}
		if got := res.Header().Get("Cache-Control"); got != tc.want {
			t.Fatalf("%s: Cache-Control %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestGo2RTCPlayerScriptIsBrieflyCacheable(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("export class VideoRTC {}"))
	}))
	defer upstream.Close()
	a := newAuthTestApp(t)
	a.Config.Go2RTCURL = upstream.URL
	res := performJSON(New(a).Handler(), http.MethodGet, "/go2rtc/video-rtc.js", nil, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Cache-Control"); got != "private, max-age=300" {
		t.Fatalf("Cache-Control %q", got)
	}
}
