package api

import (
	"errors"
	"net"
	"net/http"
	neturl "net/url"
	"strings"
	"sync"
	"time"
)

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob:; media-src 'self' blob:; connect-src 'self' ws: wss:; "+
				"font-src 'self'; frame-ancestors 'self'; base-uri 'self'; form-action 'self'; object-src 'none'")
		next.ServeHTTP(w, r)
	})
}

// withOriginCheck rejects state-changing requests whose Origin header names a
// different site. Browsers attach Origin to cross-site requests even though
// CORS lets them through for simple requests, so this blocks CSRF regardless
// of cookie SameSite behaviour.
func withOriginCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && !originMatchesRequest(origin, r.Host) {
			writeError(w, errors.New("cross-origin request abgelehnt"), http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func originMatchesRequest(origin, host string) bool {
	parsed, err := neturl.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Host, host)
}

type loginLimiter struct {
	mu    sync.Mutex
	fails map[string]*loginFailures
}

type loginFailures struct {
	count    int
	windowAt time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{fails: map[string]*loginFailures{}}
}

// Allow reports whether a login attempt from addr may proceed or the failure
// limit within the current window has been exhausted.
func (l *loginLimiter) Allow(addr string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := l.fails[clientKey(addr)]
	if f == nil || now.Sub(f.windowAt) >= loginFailWindow {
		return true
	}
	return f.count < loginFailLimit
}

func (l *loginLimiter) RecordFailure(addr string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := clientKey(addr)
	f := l.fails[key]
	if f == nil || now.Sub(f.windowAt) >= loginFailWindow {
		l.fails[key] = &loginFailures{count: 1, windowAt: now}
		return
	}
	f.count++
}

func clientKey(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

const (
	loginFailLimit  = 10
	loginFailWindow = time.Minute
)
