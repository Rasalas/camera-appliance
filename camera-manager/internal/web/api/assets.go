package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Files with these extensions may have a build-time .gz sibling (see frontend/precompress.ts).
// Explicit types avoid sniffing gzip bytes and cover extensions Go's table lacks.
var compressibleTypes = map[string]string{
	".html":        "text/html; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".svg":         "image/svg+xml",
	".json":        "application/json",
	".webmanifest": "application/manifest+json",
	".txt":         "text/plain; charset=utf-8",
	".xml":         "text/xml; charset=utf-8",
	".map":         "application/json",
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	dist := s.app.Config.FrontendDist
	clean := filepath.Clean(r.URL.Path)
	path := filepath.Join(dist, clean)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		// Vite content-hashes everything under /assets; other files keep stable names.
		if strings.HasPrefix(clean, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		serveStaticFile(w, r, path, info)
		return
	}
	index := filepath.Join(dist, "index.html")
	if info, err := os.Stat(index); err == nil {
		w.Header().Set("Cache-Control", "no-cache")
		serveStaticFile(w, r, index, info)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<html><body><h1>camera-appliance</h1><p>Frontend wurde noch nicht gebaut. Bitte npm run build im frontend-Verzeichnis ausführen.</p></body></html>`))
}

func serveStaticFile(w http.ResponseWriter, r *http.Request, path string, info os.FileInfo) {
	ctype, ok := compressibleTypes[strings.ToLower(filepath.Ext(path))]
	// ServeFile redirects explicit /index.html requests to "./"; keep that behaviour.
	if !ok || strings.HasSuffix(r.URL.Path, "/index.html") {
		http.ServeFile(w, r, path)
		return
	}
	w.Header().Set("Content-Type", ctype)
	gz, err := os.Open(path + ".gz")
	if err != nil {
		http.ServeFile(w, r, path)
		return
	}
	defer gz.Close()
	gzInfo, err := gz.Stat()
	// A .gz older than its source is stale (e.g. a hand-edited dist file); ignore it.
	if err != nil || !gzInfo.Mode().IsRegular() || gzInfo.ModTime().Truncate(time.Second).Before(info.ModTime().Truncate(time.Second)) {
		http.ServeFile(w, r, path)
		return
	}
	w.Header().Add("Vary", "Accept-Encoding")
	// Byte ranges always address the identity file so partial responses stay consistent.
	if r.Header.Get("Range") != "" || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
		http.ServeFile(w, r, path)
		return
	}
	w.Header().Set("Content-Encoding", "gzip")
	// ServeContent omits the length once Content-Encoding is set; ranges never reach here.
	w.Header().Set("Content-Length", strconv.FormatInt(gzInfo.Size(), 10))
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), gz)
}

// acceptsGzip reports whether an Accept-Encoding header allows gzip, honouring q=0.
func acceptsGzip(header string) bool {
	gzipQ, anyQ := -1.0, -1.0
	for _, part := range strings.Split(header, ",") {
		coding, params, _ := strings.Cut(part, ";")
		q := 1.0
		for _, param := range strings.Split(params, ";") {
			key, value, found := strings.Cut(strings.TrimSpace(param), "=")
			if found && strings.EqualFold(strings.TrimSpace(key), "q") {
				if parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
					q = parsed
				}
			}
		}
		switch strings.ToLower(strings.TrimSpace(coding)) {
		case "gzip", "x-gzip":
			gzipQ = q
		case "*":
			anyQ = q
		}
	}
	if gzipQ >= 0 {
		return gzipQ > 0
	}
	return anyQ > 0
}

func (s *Server) getGo2RTCAsset(w http.ResponseWriter, r *http.Request) {
	asset := strings.TrimSpace(r.PathValue("asset"))
	if asset != "video-stream.js" && asset != "video-rtc.js" {
		writeError(w, errors.New("go2rtc asset not found"), http.StatusNotFound)
		return
	}
	base, err := neturl.Parse(strings.TrimSpace(s.app.Config.Go2RTCURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		writeError(w, errors.New("go2rtc url is invalid"), http.StatusBadGateway)
		return
	}
	base.User = nil
	base.Path = strings.TrimRight(base.Path, "/") + "/" + asset
	base.RawQuery = ""
	base.Fragment = ""

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		writeError(w, err, http.StatusBadGateway)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, err, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		writeError(w, fmt.Errorf("go2rtc asset request failed: %s", resp.Status), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	// Every viewer iframe loads these; a short cache still picks up go2rtc upgrades.
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, resp.Body)
}

func (s *Server) proxyGo2RTCWebSocket(w http.ResponseWriter, r *http.Request) {
	// Browsers always send Origin on WebSocket handshakes; a missing or
	// cross-site Origin means the handshake did not come from this appliance's
	// own pages, so refuse it before any video frames can be read.
	if !originMatchesRequest(r.Header.Get("Origin"), r.Host) {
		writeError(w, errors.New("websocket-origin abgelehnt"), http.StatusForbidden)
		return
	}
	target, err := neturl.Parse(strings.TrimSpace(s.app.Config.Go2RTCURL))
	if err != nil || target.Scheme == "" || target.Host == "" {
		writeError(w, errors.New("go2rtc url is invalid"), http.StatusBadGateway)
		return
	}
	target.User = nil
	basePath := strings.TrimRight(target.Path, "/")
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = basePath + "/api/ws"
		req.URL.RawQuery = r.URL.RawQuery
		req.Host = target.Host
		req.Header.Set("Origin", target.Scheme+"://"+target.Host)
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		writeError(w, err, http.StatusBadGateway)
	}
	proxy.ServeHTTP(w, r)
}
