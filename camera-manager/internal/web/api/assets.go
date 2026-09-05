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
	"strings"
	"time"
)

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	dist := s.app.Config.FrontendDist
	path := filepath.Join(dist, filepath.Clean(r.URL.Path))
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	index := filepath.Join(dist, "index.html")
	if _, err := os.Stat(index); err == nil {
		http.ServeFile(w, r, index)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<html><body><h1>camera-appliance</h1><p>Frontend wurde noch nicht gebaut. Bitte npm run build im frontend-Verzeichnis ausführen.</p></body></html>`))
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
	w.Header().Set("Cache-Control", "no-store")
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
