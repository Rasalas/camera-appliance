package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/app"
	"camera-appliance/camera-manager/internal/cameraaccess"
)

func (s *Server) startDiscovery(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	result, err := s.app.Discover(ctx)
	writeResult(w, result, err)
}

func (s *Server) getScanRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.app.Store.ScanRuns(r.Context(), 50)
	writeResult(w, runs, err)
}

func (s *Server) getScanRun(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/discovery/runs/")
	run, err := s.app.Store.ScanRun(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, errors.New("scan run not found"), http.StatusNotFound)
		return
	}
	writeResult(w, run, err)
}

func (s *Server) getDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.app.Store.Devices(r.Context())
	writeResult(w, devices, err)
}

func (s *Server) addManualDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IP       string `json:"ip"`
		Username string `json:"username"`
		Password string `json:"password"`
		Stream   string `json:"stream"`
		Label    string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	result, err := s.app.AddManualDevice(r.Context(), app.ManualDeviceInput{
		IP:       req.IP,
		Username: req.Username,
		Stream:   req.Stream,
		Label:    req.Label,
	})
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Username) != "" || strings.TrimSpace(req.Password) != "" || strings.TrimSpace(req.Stream) != "" {
		if _, err := s.cameras.SaveCredentials(r.Context(), result.Device.ID, cameraaccess.CredentialsInput{Username: req.Username, Password: req.Password, Stream: req.Stream}); err != nil {
			writeCameraResult(w, nil, err)
			return
		}
	}
	writeJSON(w, result, http.StatusOK)
}

func (s *Server) getDevice(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/devices/")
	if strings.HasSuffix(path, "/preview") {
		writeJSON(w, map[string]string{"message": "Vorschau läuft über den go2rtc-Viewer der lokalen Oberfläche."}, http.StatusOK)
		return
	}
	if strings.HasSuffix(path, "/credentials") {
		s.getDeviceCredentials(w, r, strings.TrimSuffix(path, "/credentials"))
		return
	}
	if strings.HasSuffix(path, "/reference-image") {
		s.deviceReferenceImage(w, r, strings.TrimSuffix(path, "/reference-image"))
		return
	}
	if strings.HasSuffix(path, "/probe") {
		s.probeDevice(w, r, strings.TrimSuffix(path, "/probe"))
		return
	}
	if strings.HasSuffix(path, "/frame") {
		s.deviceFrame(w, r, strings.TrimSuffix(path, "/frame"))
		return
	}
	device, err := s.app.Store.Device(r.Context(), path)
	writeResult(w, device, err)
}

func (s *Server) deviceAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/devices/")
	if strings.HasSuffix(path, "/probe") {
		s.probeDevice(w, r, strings.TrimSuffix(path, "/probe"))
		return
	}
	if strings.HasSuffix(path, "/frame") {
		s.deviceFrame(w, r, strings.TrimSuffix(path, "/frame"))
		return
	}
	if strings.HasSuffix(path, "/credentials") {
		s.setDeviceCredentials(w, r, strings.TrimSuffix(path, "/credentials"))
		return
	}
	writeError(w, errors.New("not found"), http.StatusNotFound)
}

func (s *Server) probeDevice(w http.ResponseWriter, r *http.Request, deviceID string) {
	var req cameraaccess.CredentialsInput
	_ = json.NewDecoder(r.Body).Decode(&req)
	result, err := s.cameras.Probe(r.Context(), deviceID, req)
	writeCameraResult(w, result, err)
}

func (s *Server) deviceFrame(w http.ResponseWriter, r *http.Request, deviceID string) {
	var req cameraaccess.FrameInput
	_ = json.NewDecoder(r.Body).Decode(&req)
	result, err := s.cameras.Frame(r.Context(), deviceID, req)
	writeCameraResult(w, result, err)
}

func (s *Server) getCredentialIdentities(w http.ResponseWriter, r *http.Request) {
	result, err := s.cameras.Identities(r.Context())
	writeCameraResult(w, result, err)
}

func (s *Server) saveCredentialIdentity(w http.ResponseWriter, r *http.Request) {
	var req cameraaccess.IdentityInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	result, err := s.cameras.SaveIdentity(r.Context(), req)
	writeCameraResult(w, result, err)
}

func (s *Server) deleteCredentialIdentity(w http.ResponseWriter, r *http.Request) {
	err := s.cameras.DeleteIdentity(r.Context(), r.PathValue("id"))
	writeCameraResult(w, map[string]string{"status": "ok"}, err)
}

func (s *Server) deviceReferenceImage(w http.ResponseWriter, r *http.Request, deviceID string) {
	path, err := s.cameras.ReferenceImage(r.Context(), deviceID)
	if err != nil {
		writeCameraResult(w, nil, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=30")
	http.ServeFile(w, r, path)
}

func (s *Server) getDeviceCredentials(w http.ResponseWriter, r *http.Request, deviceID string) {
	result, err := s.cameras.Credentials(r.Context(), deviceID)
	writeCameraResult(w, result, err)
}

func (s *Server) setDeviceCredentials(w http.ResponseWriter, r *http.Request, deviceID string) {
	var req cameraaccess.CredentialsInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	result, err := s.cameras.SaveCredentials(r.Context(), deviceID, req)
	writeCameraResult(w, result, err)
}
