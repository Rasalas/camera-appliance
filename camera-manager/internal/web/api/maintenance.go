package api

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
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

// createBackup ignores client paths and secrets.env so every archive stays listable and downloadable.
func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	result, err := backup.Create(r.Context(), s.app.Config, "", false)
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
	if mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mediaType == "multipart/form-data" {
		s.restoreUploadedBackup(w, r)
		return
	}
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

// restoreUploadLimit bounds the staged archive; backups hold one SQLite file and small configs.
var restoreUploadLimit int64 = 256 << 20

// restoreUploadedBackup streams the "backup" form file to disk instead of buffering the form.
func (s *Server) restoreUploadedBackup(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, restoreUploadLimit+1<<20)
	form, err := r.MultipartReader()
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	for {
		part, err := form.NextPart()
		if errors.Is(err, io.EOF) {
			writeError(w, errors.New("Keine Sicherungsdatei hochgeladen."), http.StatusBadRequest)
			return
		}
		if err != nil {
			writeUploadError(w, err, http.StatusBadRequest)
			return
		}
		if part.FormName() != "backup" {
			_ = part.Close()
			continue
		}
		result, err := backup.RestoreUpload(r.Context(), s.app.Config, part, part.FileName(), restoreUploadLimit)
		_ = part.Close()
		if err != nil {
			writeUploadError(w, err, http.StatusInternalServerError)
			return
		}
		_ = s.app.Store.AddEvent(r.Context(), "info", "restore.completed", "Backup aus Upload wiederhergestellt", map[string]string{"file": result.Path})
		writeJSON(w, result, http.StatusOK)
		return
	}
}

func writeUploadError(w http.ResponseWriter, err error, status int) {
	var tooLarge *http.MaxBytesError
	if errors.Is(err, backup.ErrUploadTooLarge) || errors.As(err, &tooLarge) {
		writeError(w, backup.ErrUploadTooLarge, http.StatusRequestEntityTooLarge)
		return
	}
	if errors.Is(err, backup.ErrInvalidUpload) {
		status = http.StatusBadRequest
	}
	writeError(w, err, status)
}
