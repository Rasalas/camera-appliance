package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	neturl "net/url"
	"path/filepath"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/app"
	authn "camera-appliance/camera-manager/internal/auth"
)

func (s *Server) getAuthStatus(w http.ResponseWriter, r *http.Request) {
	info := authInfoFromContext(r.Context())
	status, err := s.app.AuthStatus(r.Context(), info.Role, info.ExpiresAt, info.LocalAdminBypass)
	writeResult(w, status, err)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	if !s.logins.Allow(r.RemoteAddr, time.Now()) {
		writeError(w, errors.New("zu viele fehlgeschlagene Anmeldeversuche, bitte kurz warten"), http.StatusTooManyRequests)
		return
	}
	token, result, err := s.app.Login(r.Context(), req.Username, req.Password, req.Remember)
	if err != nil {
		s.logins.RecordFailure(r.RemoteAddr, time.Now())
		writeError(w, err, http.StatusUnauthorized)
		return
	}
	setSessionCookie(w, r, token, result.ExpiresAt, req.Remember)
	writeJSON(w, result, http.StatusOK)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.app.Logout(r.Context(), cookie.Value)
	}
	clearSessionCookie(w, r)
	writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (s *Server) setAuthPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	info := authInfoFromContext(r.Context())
	status, err := s.app.AuthStatus(r.Context(), info.Role, info.ExpiresAt, info.LocalAdminBypass)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	if status.Enabled && info.Role != authn.RoleAdmin {
		writeError(w, errors.New("admin login required"), unauthorizedStatus(info.Role, authn.RoleAdmin))
		return
	}
	if err := s.app.SetAuthPassword(r.Context(), req.Role, req.Password); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

const sessionCookieName = "camera_appliance_session"

type authContextKey struct{}

type requestAuthInfo struct {
	Role             string
	ExpiresAt        *time.Time
	LocalAdminBypass bool
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := s.requestAuthInfo(r)
		ctx := context.WithValue(r.Context(), authContextKey{}, info)
		r = r.WithContext(ctx)

		status, err := s.app.AuthStatus(r.Context(), info.Role, info.ExpiresAt, info.LocalAdminBypass)
		if err != nil {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				writeError(w, err, http.StatusInternalServerError)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if !status.Enabled {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/go2rtc/") {
			if isPublicAuthAPI(r) {
				next.ServeHTTP(w, r)
				return
			}
			requiredRole := requiredAPIRole(r)
			if requiredRole == authn.RoleViewer && status.ViewerPublic {
				next.ServeHTTP(w, r)
				return
			}
			if roleAllowed(info.Role, requiredRole) {
				next.ServeHTTP(w, r)
				return
			}
			writeError(w, errors.New("login required"), unauthorizedStatus(info.Role, requiredRole))
			return
		}
		if shouldRedirectStaticToLogin(r.URL.Path, status, info) {
			nextParam := r.URL.RequestURI()
			http.Redirect(w, r, "/login?next="+neturl.QueryEscape(nextParam), http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestAuthInfo(r *http.Request) requestAuthInfo {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		session, sessionErr := s.app.AuthSession(r.Context(), cookie.Value)
		if sessionErr == nil && authn.IsRole(session.Role) {
			expiresAt := session.ExpiresAt
			return requestAuthInfo{Role: session.Role, ExpiresAt: &expiresAt}
		}
	}
	status, err := s.app.AuthStatus(r.Context(), "", nil, false)
	if err == nil && !status.Enabled {
		return requestAuthInfo{Role: authn.RoleAdmin}
	}
	if err == nil && status.LocalAdminBypass && isLoopbackRemote(r.RemoteAddr) && isLoopbackHost(r.Host) {
		return requestAuthInfo{Role: authn.RoleAdmin, LocalAdminBypass: true}
	}
	return requestAuthInfo{}
}

// isLoopbackHost reports whether the request's Host header points at the local
// machine. This blocks DNS-rebinding attempts: an attacker page can make
// requests that physically originate from loopback, but their Host header will
// name the attacker's domain instead of a loopback address.
func isLoopbackHost(host string) bool {
	if host == "" {
		return false
	}
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	hostname = strings.Trim(hostname, "[]")
	if ip := net.ParseIP(hostname); ip != nil {
		return ip.IsLoopback()
	}
	return strings.EqualFold(hostname, "localhost")
}

func authInfoFromContext(ctx context.Context) requestAuthInfo {
	info, _ := ctx.Value(authContextKey{}).(requestAuthInfo)
	return info
}

func isPublicAuthAPI(r *http.Request) bool {
	switch r.URL.Path {
	case "/api/auth/status", "/api/health":
		return r.Method == http.MethodGet
	case "/api/auth/login", "/api/auth/logout", "/api/auth/password":
		return r.Method == http.MethodPost
	default:
		return false
	}
}

func requiredAPIRole(r *http.Request) string {
	if r.Method == http.MethodGet && (r.URL.Path == "/api/viewer" || strings.HasPrefix(r.URL.Path, "/go2rtc/")) {
		return authn.RoleViewer
	}
	return authn.RoleAdmin
}

func roleAllowed(role, requiredRole string) bool {
	if requiredRole == authn.RoleViewer {
		return role == authn.RoleViewer || role == authn.RoleAdmin
	}
	return role == authn.RoleAdmin
}

func unauthorizedStatus(role, requiredRole string) int {
	if role != "" && !roleAllowed(role, requiredRole) {
		return http.StatusForbidden
	}
	return http.StatusUnauthorized
}

func shouldRedirectStaticToLogin(path string, status app.AuthStatus, info requestAuthInfo) bool {
	if path == "/login" || isStaticAssetPath(path) {
		return false
	}
	if isAdminUIPath(path) {
		return info.Role != authn.RoleAdmin
	}
	if path == "/" && !status.ViewerPublic {
		return info.Role != authn.RoleAdmin && info.Role != authn.RoleViewer
	}
	return false
}

func isStaticAssetPath(path string) bool {
	if strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/fonts/") {
		return true
	}
	return filepath.Ext(path) != ""
}

func isAdminUIPath(path string) bool {
	for _, prefix := range []string{
		"/einrichtung",
		"/uebersicht",
		"/system",
		"/kamera",
		"/setup",
		"/overview",
		"/cameras",
		"/discovery",
		"/assign",
		"/bindings",
		"/devices",
		"/settings",
		"/events",
		"/backup",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func isLoopbackRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time, remember bool) {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	}
	if remember {
		cookie.Expires = expiresAt
		cookie.MaxAge = max(1, int(time.Until(expiresAt).Seconds()))
	}
	http.SetCookie(w, cookie)
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
}
