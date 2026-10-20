package api

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func writeGzipFixture(t *testing.T, path string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func staticRequest(handler http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func TestStaticServesPrecompressedGzip(t *testing.T) {
	a := newAuthTestApp(t)
	dist := t.TempDir()
	a.Config.FrontendDist = dist
	if err := os.MkdirAll(filepath.Join(dist, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	js := []byte(strings.Repeat("export const kamera = 1;\n", 100))
	html := []byte("<html><body>" + strings.Repeat("<p>Kamera</p>", 100) + "</body></html>")
	font := []byte(strings.Repeat("\x00", 2048))
	for name, body := range map[string][]byte{
		"assets/index-AbC1.js":   js,
		"index.html":             html,
		"assets/font-AbC1.woff2": font,
		"assets/plain-AbC1.css":  []byte("a{color:red}"),
		"assets/stale-AbC1.js":   js,
		"manifest.webmanifest":   []byte(`{"name":"Kameras"}`),
	} {
		if err := os.WriteFile(filepath.Join(dist, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	jsGz := writeGzipFixture(t, filepath.Join(dist, "assets/index-AbC1.js.gz"), js)
	writeGzipFixture(t, filepath.Join(dist, "index.html.gz"), html)
	writeGzipFixture(t, filepath.Join(dist, "assets/font-AbC1.woff2.gz"), font)
	writeGzipFixture(t, filepath.Join(dist, "manifest.webmanifest.gz"), []byte(`{"name":"Kameras"}`))
	stale := filepath.Join(dist, "assets/stale-AbC1.js.gz")
	writeGzipFixture(t, stale, js)
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	handler := New(a).Handler()
	gzipOK := map[string]string{"Accept-Encoding": "gzip, deflate"}

	t.Run("gzip variant for accepting client", func(t *testing.T) {
		res := staticRequest(handler, http.MethodGet, "/assets/index-AbC1.js", gzipOK)
		if res.Code != http.StatusOK || res.Header().Get("Content-Encoding") != "gzip" {
			t.Fatalf("status %d encoding %q", res.Code, res.Header().Get("Content-Encoding"))
		}
		h := res.Header()
		if h.Get("Vary") != "Accept-Encoding" || h.Get("Content-Type") != "text/javascript; charset=utf-8" ||
			h.Get("Cache-Control") != "public, max-age=31536000, immutable" || h.Get("Content-Length") != strconv.Itoa(len(jsGz)) {
			t.Fatalf("headers %v", h)
		}
		zr, err := gzip.NewReader(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(zr)
		if err != nil || !bytes.Equal(got, js) {
			t.Fatalf("decoded body mismatch: %v", err)
		}
	})

	t.Run("identity when gzip is not accepted", func(t *testing.T) {
		for _, ae := range []string{"", "identity", "gzip;q=0", "br"} {
			res := staticRequest(handler, http.MethodGet, "/assets/index-AbC1.js", map[string]string{"Accept-Encoding": ae})
			if res.Header().Get("Content-Encoding") != "" || !bytes.Equal(res.Body.Bytes(), js) {
				t.Fatalf("%q: encoding %q", ae, res.Header().Get("Content-Encoding"))
			}
			if res.Header().Get("Vary") != "Accept-Encoding" {
				t.Fatalf("%q: Vary %q", ae, res.Header().Get("Vary"))
			}
		}
	})

	t.Run("HEAD reports the gzip representation", func(t *testing.T) {
		res := staticRequest(handler, http.MethodHead, "/assets/index-AbC1.js", gzipOK)
		if res.Code != http.StatusOK || res.Header().Get("Content-Encoding") != "gzip" || res.Body.Len() != 0 ||
			res.Header().Get("Content-Length") != strconv.Itoa(len(jsGz)) {
			t.Fatalf("status %d headers %v body %d", res.Code, res.Header(), res.Body.Len())
		}
	})

	t.Run("revalidation returns 304 without representation headers", func(t *testing.T) {
		since := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)
		res := staticRequest(handler, http.MethodGet, "/", map[string]string{"Accept-Encoding": "gzip", "If-Modified-Since": since})
		h := res.Header()
		if res.Code != http.StatusNotModified || h.Get("Content-Length") != "" || h.Get("Content-Encoding") != "" || h.Get("Cache-Control") != "no-cache" {
			t.Fatalf("status %d headers %v", res.Code, h)
		}
	})

	t.Run("range requests use the identity file", func(t *testing.T) {
		res := staticRequest(handler, http.MethodGet, "/assets/index-AbC1.js",
			map[string]string{"Accept-Encoding": "gzip", "Range": "bytes=0-5"})
		if res.Code != http.StatusPartialContent || res.Header().Get("Content-Encoding") != "" || res.Body.String() != string(js[:6]) {
			t.Fatalf("status %d encoding %q body %q", res.Code, res.Header().Get("Content-Encoding"), res.Body.String())
		}
	})

	t.Run("index and SPA fallback stay no-cache", func(t *testing.T) {
		for _, path := range []string{"/", "/kamera/cam1"} {
			res := staticRequest(handler, http.MethodGet, path, gzipOK)
			h := res.Header()
			if res.Code != http.StatusOK || h.Get("Content-Encoding") != "gzip" || h.Get("Cache-Control") != "no-cache" ||
				h.Get("Content-Type") != "text/html; charset=utf-8" {
				t.Fatalf("%s: status %d headers %v", path, res.Code, h)
			}
		}
		res := staticRequest(handler, http.MethodGet, "/index.html", gzipOK)
		if res.Code != http.StatusMovedPermanently {
			t.Fatalf("/index.html: status %d", res.Code)
		}
	})

	t.Run("non-compressible, missing or stale variants are ignored", func(t *testing.T) {
		for _, path := range []string{"/assets/font-AbC1.woff2", "/assets/plain-AbC1.css", "/assets/stale-AbC1.js"} {
			res := staticRequest(handler, http.MethodGet, path, gzipOK)
			if res.Code != http.StatusOK || res.Header().Get("Content-Encoding") != "" || res.Header().Get("Vary") != "" {
				t.Fatalf("%s: status %d headers %v", path, res.Code, res.Header())
			}
		}
		res := staticRequest(handler, http.MethodGet, "/assets/plain-AbC1.css", gzipOK)
		if res.Header().Get("Content-Type") != "text/css; charset=utf-8" {
			t.Fatalf("css Content-Type %q", res.Header().Get("Content-Type"))
		}
	})

	t.Run("types missing from Go's table are not sniffed from gzip bytes", func(t *testing.T) {
		res := staticRequest(handler, http.MethodGet, "/manifest.webmanifest", gzipOK)
		if res.Header().Get("Content-Encoding") != "gzip" || res.Header().Get("Content-Type") != "application/manifest+json" {
			t.Fatalf("headers %v", res.Header())
		}
	})

	t.Run("API responses are not compressed", func(t *testing.T) {
		res := staticRequest(handler, http.MethodGet, "/api/auth/status", gzipOK)
		if res.Header().Get("Content-Encoding") != "" || res.Header().Get("Vary") != "" {
			t.Fatalf("headers %v", res.Header())
		}
	})
}

func TestAcceptsGzip(t *testing.T) {
	for header, want := range map[string]bool{
		"":                   false,
		"gzip":               true,
		"gzip, deflate, br":  true,
		"GZIP;q=0.5":         true,
		"x-gzip":             true,
		"gzip;q=0":           false,
		"gzip; q=0.000":      false,
		"*":                  true,
		"br, *;q=0.1":        true,
		"*;q=0":              false,
		"*, gzip;q=0":        false,
		"identity":           false,
		"deflate, br;q=1.0":  false,
		" gzip ; q = 1 , br": true,
	} {
		if got := acceptsGzip(header); got != want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", header, got, want)
		}
	}
}

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
