package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"camera-appliance/camera-manager/internal/cameraaccess"
	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/state"
)

func writeResult(w http.ResponseWriter, value any, err error) {
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, state.ErrInvalidBinding) {
			status = http.StatusBadRequest
		}
		writeError(w, err, status)
		return
	}
	writeJSON(w, value, http.StatusOK)
}

func writeJSON(w http.ResponseWriter, value any, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error, status int) {
	writeJSON(w, map[string]string{"error": redaction.Text(err.Error())}, status)
}

func writeCameraResult(w http.ResponseWriter, result any, err error) {
	if err == nil {
		writeJSON(w, result, http.StatusOK)
		return
	}
	status := http.StatusInternalServerError
	var failure *cameraaccess.Failure
	if errors.As(err, &failure) {
		switch failure.Kind {
		case cameraaccess.InvalidInput:
			status = http.StatusBadRequest
		case cameraaccess.NotFound:
			status = http.StatusNotFound
		case cameraaccess.CaptureFailed:
			status = http.StatusBadGateway
		}
	}
	writeError(w, err, status)
}
