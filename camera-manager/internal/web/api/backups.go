package api

import (
	"errors"
	"mime"
	"net/http"

	"camera-appliance/camera-manager/internal/app"
	"camera-appliance/camera-manager/internal/backup"
)

type backupList struct {
	Backups   []backup.Entry       `json:"backups"`
	Auto      app.AutoBackupStatus `json:"auto"`
	Retention map[backup.Kind]int  `json:"retention"`
	Warning   string               `json:"warning"`
}

func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	entries, err := backup.List(s.app.Config)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	auto, err := s.app.AutoBackupStatus(r.Context(), entries)
	w.Header().Set("Cache-Control", "no-store")
	retention := map[backup.Kind]int{}
	for _, kind := range []backup.Kind{backup.KindManual, backup.KindUpdate, backup.KindAutomatic} {
		retention[kind] = backup.Keep(kind)
	}
	writeResult(w, backupList{Backups: entries, Auto: auto, Retention: retention, Warning: backup.SensitiveWarning}, err)
}

func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	file, entry, err := backup.Open(s.app.Config, r.PathValue("name"))
	if err != nil {
		writeBackupError(w, err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": entry.Name}))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, entry.Name, entry.CreatedAt, file)
}

func (s *Server) deleteBackup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := backup.Delete(s.app.Config, name); err != nil {
		writeBackupError(w, err)
		return
	}
	_ = s.app.Store.AddEvent(r.Context(), "info", "backup.deleted", "Sicherung gelöscht", map[string]string{"file": name})
	writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func writeBackupError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, backup.ErrInvalidName):
		writeError(w, err, http.StatusBadRequest)
	case errors.Is(err, backup.ErrNotFound):
		writeError(w, err, http.StatusNotFound)
	default:
		writeError(w, err, http.StatusInternalServerError)
	}
}
