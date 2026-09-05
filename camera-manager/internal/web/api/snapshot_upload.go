package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"camera-appliance/camera-manager/internal/snapshotupload"
)

func decodeUpload(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		writeError(w, errors.New("Ungültige Upload-Anfrage."), http.StatusBadRequest)
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeError(w, errors.New("Ungültige Upload-Anfrage."), http.StatusBadRequest)
		return false
	}
	return true
}

func writeUploadResult(w http.ResponseWriter, value any, err error) {
	switch {
	case errors.Is(err, snapshotupload.ErrInvalid):
		writeError(w, err, http.StatusBadRequest)
	case errors.Is(err, snapshotupload.ErrBusy):
		writeError(w, err, http.StatusConflict)
	case errors.Is(err, snapshotupload.ErrRemote):
		writeError(w, err, http.StatusBadGateway)
	default:
		writeCameraResult(w, value, err)
	}
}

func (s *Server) getUploadSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	value, err := s.uploads.Settings(r.Context())
	writeUploadResult(w, value, err)
}

func (s *Server) putUploadSettings(w http.ResponseWriter, r *http.Request) {
	var input snapshotupload.SettingsInput
	if !decodeUpload(w, r, &input) {
		return
	}
	value, err := s.uploads.SaveSettings(r.Context(), input)
	writeUploadResult(w, value, err)
}

func (s *Server) getUploadCrop(w http.ResponseWriter, r *http.Request) {
	value, err := s.uploads.Crop(r.Context(), r.PathValue("id"))
	writeUploadResult(w, value, err)
}

func (s *Server) putUploadCrop(w http.ResponseWriter, r *http.Request) {
	var input snapshotupload.Crop
	if !decodeUpload(w, r, &input) {
		return
	}
	err := s.uploads.SaveCrop(r.Context(), r.PathValue("id"), input)
	writeUploadResult(w, input, err)
}

func (s *Server) uploadSnapshot(w http.ResponseWriter, r *http.Request) {
	var input snapshotupload.UploadInput
	if !decodeUpload(w, r, &input) {
		return
	}
	result, err := s.uploads.Upload(r.Context(), r.PathValue("id"), input)
	if err == nil {
		_ = s.app.Store.AddEvent(r.Context(), "info", "snapshot.uploaded", "Kamera-Einzelbild hochgeladen", map[string]string{"device_id": r.PathValue("id"), "filename": result.Filename})
	}
	writeUploadResult(w, result, err)
}

func (s *Server) getUploadSchedule(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	result, err := s.uploads.GetSchedule(r.Context(), r.PathValue("id"))
	writeUploadResult(w, result, err)
}

func (s *Server) putUploadSchedule(w http.ResponseWriter, r *http.Request) {
	var input snapshotupload.ScheduleInput
	if !decodeUpload(w, r, &input) {
		return
	}
	result, err := s.uploads.SaveSchedule(r.Context(), r.PathValue("id"), input)
	writeUploadResult(w, result, err)
}

func (s *Server) getUploadNaming(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	result, err := s.uploads.GetNaming(r.Context(), r.PathValue("id"))
	writeUploadResult(w, result, err)
}

func (s *Server) putUploadNaming(w http.ResponseWriter, r *http.Request) {
	var input snapshotupload.Naming
	if !decodeUpload(w, r, &input) {
		return
	}
	result, err := s.uploads.SaveNaming(r.Context(), r.PathValue("id"), input)
	writeUploadResult(w, result, err)
}

func (s *Server) getUploadImageSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	value, err := s.uploads.GetImageSettings(r.Context(), r.PathValue("id"))
	writeUploadResult(w, value, err)
}
func (s *Server) putUploadImageSettings(w http.ResponseWriter, r *http.Request) {
	var input snapshotupload.ImageSettings
	if !decodeUpload(w, r, &input) {
		return
	}
	value, err := s.uploads.SaveImageSettings(r.Context(), r.PathValue("id"), input)
	writeUploadResult(w, value, err)
}
