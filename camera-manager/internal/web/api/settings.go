package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"camera-appliance/camera-manager/internal/app"
	"camera-appliance/camera-manager/internal/secrets"
)

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.app.Store.Settings(r.Context())
	if settings == nil {
		settings = map[string]string{}
	}
	delete(settings, app.AuthSettingAdminPasswordHash)
	delete(settings, app.AuthSettingViewerPasswordHash)
	settings["go2rtc_url"] = s.app.Config.Go2RTCURL
	settings["bind_addr"] = s.app.Config.BindAddr
	if settings[app.NetworkSettingLANAccess] == "" {
		settings[app.NetworkSettingLANAccess] = fmt.Sprintf("%t", app.LANAccessEnabled(s.app.Config.BindAddr))
	}
	if settings["capture_ssh_host"] == "" {
		settings["capture_ssh_host"] = s.app.Config.CaptureSSHHost
	}
	activePassword, activeSource := s.app.CameraCredentials()
	settings["camera_password_set"] = fmt.Sprintf("%t", activePassword != "")
	settings["camera_password_source"] = activeSource
	info := authInfoFromContext(r.Context())
	authStatus, authErr := s.app.AuthStatus(r.Context(), info.Role, info.ExpiresAt, info.LocalAdminBypass)
	if authErr == nil {
		settings["auth_admin_password_set"] = fmt.Sprintf("%t", authStatus.AdminPasswordSet)
		settings["auth_viewer_password_set"] = fmt.Sprintf("%t", authStatus.ViewerPasswordSet)
		if settings[app.AuthSettingViewerPublic] == "" {
			settings[app.AuthSettingViewerPublic] = fmt.Sprintf("%t", authStatus.ViewerPublic)
		}
		if settings[app.AuthSettingLocalAdminBypass] == "" {
			settings[app.AuthSettingLocalAdminBypass] = fmt.Sprintf("%t", authStatus.LocalAdminBypass)
		}
		if settings[app.AuthSettingSessionHours] == "" {
			settings[app.AuthSettingSessionHours] = fmt.Sprintf("%d", authStatus.SessionHours)
		}
	}
	writeResult(w, settings, err)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var settings map[string]string
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	if err := s.app.UpdateSettings(r.Context(), settings); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (s *Server) setCameraPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	source, err := secrets.Save(s.app.Config.ConfigDir, req.Password)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	s.app.SetCameraCredentials(strings.TrimSpace(req.Password), source)
	_ = s.app.Store.AddEvent(r.Context(), "info", "settings.secret.updated", "Kamera-Passwort wurde gespeichert", map[string]string{"source": source})
	writeJSON(w, map[string]string{"status": "ok", "source": source}, http.StatusOK)
}
