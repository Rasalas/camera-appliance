package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	neturl "net/url"
	"strings"

	"camera-appliance/camera-manager/internal/backup"
	"camera-appliance/camera-manager/internal/system"
	updater "camera-appliance/camera-manager/internal/update"
)

func (s *Server) restartStack(w http.ResponseWriter, r *http.Request) {
	err := system.RestartStack(r.Context(), s.app.Config)
	writeResult(w, map[string]string{"status": "ok"}, err)
}

func (s *Server) startUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL    string `json:"url"`
		Digest string `json:"digest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	updateURL := strings.TrimSpace(req.URL)
	if updateURL == "" {
		updateURL = updater.DefaultReleaseURL
	}
	parsed, err := neturl.ParseRequestURI(updateURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		writeError(w, errors.New("update url is invalid"), http.StatusBadRequest)
		return
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && insecureUpdateAllowed()) {
		writeError(w, errors.New("update url muss https verwenden"), http.StatusBadRequest)
		return
	}
	s.submitUpdate(w, r, updater.Request{URL: updateURL, Digest: strings.TrimSpace(req.Digest), AutoRollback: true, AllowInsecureURL: insecureUpdateAllowed()})
}

func (s *Server) getEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.app.Store.Events(r.Context(), 100)
	writeResult(w, events, err)
}

func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Out            string `json:"out"`
		IncludeSecrets bool   `json:"include_secrets"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	result, err := backup.Create(r.Context(), s.app.Config, req.Out, req.IncludeSecrets)
	if err == nil {
		_ = s.app.Store.AddEvent(r.Context(), "info", "backup.created", "Backup erstellt", map[string]string{"path": result.Path})
	}
	writeResult(w, result, err)
}

func (s *Server) createSupportBundle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Out string `json:"out"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	result, err := s.app.CreateSupportBundle(r.Context(), req.Out)
	writeResult(w, result, err)
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		In string `json:"in"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	result, err := backup.Restore(r.Context(), s.app.Config, req.In)
	if err == nil {
		_ = s.app.Store.AddEvent(r.Context(), "info", "restore.completed", "Backup wiederhergestellt", map[string]string{"path": result.Path})
	}
	writeResult(w, result, err)
}
