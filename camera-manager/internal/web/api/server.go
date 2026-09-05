package api

import (
	"context"
	"net/http"
	"path/filepath"

	"camera-appliance/camera-manager/internal/app"
	"camera-appliance/camera-manager/internal/cameraaccess"
	"camera-appliance/camera-manager/internal/config"
	updater "camera-appliance/camera-manager/internal/update"
	"camera-appliance/camera-manager/internal/version"
)

type Server struct {
	cameras        *cameraaccess.Service
	app            *app.App
	mux            *http.ServeMux
	logins         *loginLimiter
	startUpdateJob func(context.Context, config.Config, updater.Request) (updater.Job, error)
	updates        *updateFlow
}

func New(a *app.App) *Server {
	s := &Server{
		cameras:        cameraaccess.New(a.Store, a.Config, a.StreamEndpointForDevice),
		app:            a,
		mux:            http.NewServeMux(),
		logins:         newLoginLimiter(),
		startUpdateJob: updater.StartJob,
		updates:        newUpdateFlow(filepath.Join(a.Config.StateDir, "updates")),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return withSecurityHeaders(withOriginCheck(s.authMiddleware(s.mux)))
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/auth/status", s.getAuthStatus)
	s.mux.HandleFunc("GET /api/health", s.getHealth)
	s.mux.HandleFunc("POST /api/auth/login", s.login)
	s.mux.HandleFunc("POST /api/auth/logout", s.logout)
	s.mux.HandleFunc("POST /api/auth/password", s.setAuthPassword)
	s.mux.HandleFunc("GET /api/status", s.getStatus)
	s.mux.HandleFunc("POST /api/discovery/start", s.startDiscovery)
	s.mux.HandleFunc("GET /api/discovery/runs", s.getScanRuns)
	s.mux.HandleFunc("GET /api/discovery/runs/", s.getScanRun)
	s.mux.HandleFunc("GET /api/devices", s.getDevices)
	s.mux.HandleFunc("POST /api/devices/manual", s.addManualDevice)
	s.mux.HandleFunc("POST /api/devices/{rest...}", s.deviceAction)
	s.mux.HandleFunc("GET /api/devices/{rest...}", s.getDevice)
	s.mux.HandleFunc("GET /api/slots", s.getSlots)
	s.mux.HandleFunc("GET /api/bindings", s.getBindings)
	s.mux.HandleFunc("POST /api/bindings", s.postBinding)
	s.mux.HandleFunc("DELETE /api/bindings/", s.deleteBinding)
	s.mux.HandleFunc("POST /api/bindings/", s.replaceBinding)
	s.mux.HandleFunc("POST /api/go2rtc/render", s.renderGo2RTC)
	s.mux.HandleFunc("POST /api/go2rtc/restart", s.restartGo2RTC)
	s.mux.HandleFunc("GET /api/relays/status", s.getRelayStatus)
	s.mux.HandleFunc("POST /api/relays/{id}/start", s.startRelay)
	s.mux.HandleFunc("POST /api/relays/{id}/stop", s.stopRelay)
	s.mux.HandleFunc("POST /api/relays/{id}/restart", s.restartRelay)
	s.mux.HandleFunc("GET /api/viewer", s.getViewer)
	s.mux.HandleFunc("POST /api/system/restart-stack", s.restartStack)
	s.mux.HandleFunc("POST /api/system/update", s.startUpdate)
	s.mux.HandleFunc("GET /api/system/update/status", s.getUpdateStatus)
	s.mux.HandleFunc("POST /api/system/update/check", s.checkForUpdate)
	s.mux.HandleFunc("POST /api/system/update/download", s.downloadUpdate)
	s.mux.HandleFunc("POST /api/system/update/install", s.startUpdateInstall)
	s.mux.HandleFunc("GET /api/credential-identities", s.getCredentialIdentities)
	s.mux.HandleFunc("POST /api/credential-identities", s.saveCredentialIdentity)
	s.mux.HandleFunc("DELETE /api/credential-identities/{id}", s.deleteCredentialIdentity)
	s.mux.HandleFunc("GET /api/settings", s.getSettings)
	s.mux.HandleFunc("PUT /api/settings", s.putSettings)
	s.mux.HandleFunc("POST /api/secrets/camera-password", s.setCameraPassword)
	s.mux.HandleFunc("GET /api/events", s.getEvents)
	s.mux.HandleFunc("POST /api/backup", s.createBackup)
	s.mux.HandleFunc("POST /api/support-bundle", s.createSupportBundle)
	s.mux.HandleFunc("POST /api/restore", s.restoreBackup)
	s.mux.HandleFunc("GET /go2rtc/api/ws", s.proxyGo2RTCWebSocket)
	s.mux.HandleFunc("GET /go2rtc/{asset}", s.getGo2RTCAsset)
	s.mux.HandleFunc("/", s.static)
}

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.app.Status(r.Context())
	writeResult(w, status, err)
}

func (s *Server) getViewer(w http.ResponseWriter, r *http.Request) {
	viewer, err := s.app.Viewer(r.Context())
	writeResult(w, viewer, err)
}

// getHealth identifies the running release without exposing deployment paths.
// It stays public so post-update healthchecks work with auth enabled.
func (s *Server) getHealth(w http.ResponseWriter, _ *http.Request) {
	current := version.Current()
	writeJSON(w, map[string]string{"status": "ok", "version": current.Version, "commit": current.Commit}, http.StatusOK)
}
