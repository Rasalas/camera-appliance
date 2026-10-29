package api

import (
	"camera-appliance/camera-manager/internal/app"
	"camera-appliance/camera-manager/internal/state"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func (s *Server) putDeviceConfiguration(w http.ResponseWriter, r *http.Request) {
	var req app.CameraConfiguration
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, errors.New("Ungültige Kamera-Konfiguration."), http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeError(w, errors.New("Ungültige Kamera-Konfiguration."), http.StatusBadRequest)
		return
	}
	result, err := s.app.ConfigureCamera(r.Context(), r.PathValue("id"), req)
	status := http.StatusOK
	switch {
	case errors.Is(err, sql.ErrNoRows):
		status = http.StatusNotFound
		result.Error = "Kamera nicht gefunden."
	case errors.Is(err, state.ErrNoCameraSlot):
		status = http.StatusConflict
		result.Error = err.Error()
	case errors.Is(err, app.ErrInvalidCameraConfiguration), errors.Is(err, state.ErrInvalidDeviceName):
		status = http.StatusBadRequest
		result.Error = err.Error()
	case err != nil:
		status = http.StatusInternalServerError
		result.Error = "Kamera-Konfiguration konnte nicht vollständig gespeichert werden."
	}
	writeJSON(w, result, status)
}
