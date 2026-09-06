package api

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/state"
	"camera-appliance/camera-manager/internal/version"
)

func (s *Server) getSupportReport(w http.ResponseWriter, r *http.Request) {
	events, err := s.app.Store.Events(r.Context(), 100)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	for i := range events {
		events[i].Message = redaction.Text(events[i].Message)
		events[i].Type = redaction.Text(events[i].Type)
		events[i].Level = redaction.Text(events[i].Level)
		events[i].DetailsJSON = nil
	}
	w.Header().Set("Cache-Control", "no-store")
	writeResult(w, struct {
		Version version.Info  `json:"version"`
		Events  []state.Event `json:"events"`
	}{version.Current(), events}, nil)
}

// Generate a fresh, redacted bundle. Clients never supply a filesystem path.
func (s *Server) downloadSupportBundle(w http.ResponseWriter, r *http.Request) {
	if err := os.MkdirAll(s.app.Config.BackupDir(), 0o750); err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	dir, err := os.MkdirTemp(s.app.Config.BackupDir(), "support-download-")
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(dir)
	name := "support-bundle-" + time.Now().UTC().Format("20060102-150405") + ".tar.gz"
	result, err := s.app.CreateSupportBundle(r.Context(), filepath.Join(dir, name))
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	file, err := os.Open(result.Path)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, name, time.Time{}, file)
}
