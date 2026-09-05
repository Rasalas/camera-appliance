package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"camera-appliance/camera-manager/internal/app"
	authn "camera-appliance/camera-manager/internal/auth"
	"camera-appliance/camera-manager/internal/backup"
	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/secrets"
	"camera-appliance/camera-manager/internal/state"
	"camera-appliance/camera-manager/internal/system"
	updater "camera-appliance/camera-manager/internal/update"
)

type Server struct {
	app      *app.App
	mux      *http.ServeMux
	logins   *loginLimiter
	updateMu sync.Mutex
	updates  *updateFlow
}

type credentialIdentity struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Username       string `json:"username"`
	PasswordSet    bool   `json:"password_set"`
	PasswordSource string `json:"password_source,omitempty"`
}

type credentialCandidate struct {
	Source     string
	IdentityID string
	Username   string
	Password   string
	Stream     string
}

const (
	credentialIdentityIDsKey = "camera.identity.ids"
)

func New(a *app.App) *Server {
	s := &Server{
		app:     a,
		mux:     http.NewServeMux(),
		logins:  newLoginLimiter(),
		updates: newUpdateFlow(filepath.Join(a.Config.StateDir, "updates")),
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

func (s *Server) getAuthStatus(w http.ResponseWriter, r *http.Request) {
	info := authInfoFromContext(r.Context())
	status, err := s.app.AuthStatus(r.Context(), info.Role, info.ExpiresAt, info.LocalAdminBypass)
	writeResult(w, status, err)
}

// getHealth serves a minimal liveness probe without any deployment details.
// It stays public so post-update healthchecks work with auth enabled.
func (s *Server) getHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	if !s.logins.Allow(r.RemoteAddr, time.Now()) {
		writeError(w, errors.New("zu viele fehlgeschlagene Anmeldeversuche, bitte kurz warten"), http.StatusTooManyRequests)
		return
	}
	token, result, err := s.app.Login(r.Context(), req.Username, req.Password, req.Remember)
	if err != nil {
		s.logins.RecordFailure(r.RemoteAddr, time.Now())
		writeError(w, err, http.StatusUnauthorized)
		return
	}
	setSessionCookie(w, r, token, result.ExpiresAt, req.Remember)
	writeJSON(w, result, http.StatusOK)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.app.Logout(r.Context(), cookie.Value)
	}
	clearSessionCookie(w, r)
	writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (s *Server) setAuthPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	info := authInfoFromContext(r.Context())
	status, err := s.app.AuthStatus(r.Context(), info.Role, info.ExpiresAt, info.LocalAdminBypass)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	if status.Enabled && info.Role != authn.RoleAdmin {
		writeError(w, errors.New("admin login required"), unauthorizedStatus(info.Role, authn.RoleAdmin))
		return
	}
	if err := s.app.SetAuthPassword(r.Context(), req.Role, req.Password); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

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
		if req.Stream == "" {
			req.Stream = "stream2"
		}
		// Persist the password first: a failure here must not leave a saved
		// username that silently pairs with no credentials.
		if strings.TrimSpace(req.Password) != "" {
			if _, err := secrets.SaveCamera(s.app.Config.ConfigDir, result.Device.ID, req.Password); err != nil {
				writeError(w, err, http.StatusInternalServerError)
				return
			}
		}
		values := map[string]string{
			"camera.credentials." + result.Device.ID + ".username": strings.TrimSpace(req.Username),
			"camera.credentials." + result.Device.ID + ".stream":   strings.TrimSpace(req.Stream),
		}
		if err := s.app.Store.PutSettings(r.Context(), values); err != nil {
			writeError(w, err, http.StatusInternalServerError)
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
	if r.Method != http.MethodPost {
		writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	device, err := s.app.Store.Device(r.Context(), deviceID)
	if err != nil {
		writeError(w, err, http.StatusNotFound)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Stream   string `json:"stream"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	applySavedCredentials(r.Context(), s, deviceID, &req.Username, &req.Password, &req.Stream)
	if req.Stream == "" {
		req.Stream = "stream2"
	}
	endpoint, err := s.app.StreamEndpointForDevice(r.Context(), device)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	rawURL := cameraRTSPURL(req.Username, req.Password, endpoint.Host, endpoint.Port, req.Stream)
	ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
	defer cancel()
	conn, dialErr := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(app.ProbeHostForEndpoint(endpoint.Host), endpoint.Port))
	if dialErr == nil {
		_ = conn.Close()
	}
	writeJSON(w, map[string]any{
		"success":      dialErr == nil,
		"url_redacted": redaction.URL(rawURL),
		"message":      probeMessage(dialErr),
	}, http.StatusOK)
}

func probeMessage(err error) string {
	if err == nil {
		return "RTSP-Port erreichbar. Passwort wird beim Vorschaubild oder go2rtc-Stream praktisch geprüft."
	}
	return "RTSP-Port nicht erreichbar. Prüfe Netzwerk, Stromversorgung oder ob RTSP/ONVIF aktiv ist."
}

func applySavedCredentials(ctx context.Context, s *Server, deviceID string, username, password, stream *string) {
	settings, _ := s.app.Store.Settings(ctx)
	if strings.TrimSpace(*username) == "" {
		*username = settings["camera.credentials."+deviceID+".username"]
	}
	if strings.TrimSpace(*stream) == "" {
		*stream = settings["camera.credentials."+deviceID+".stream"]
	}
	if strings.TrimSpace(*password) == "" {
		*password = secrets.LoadCamera(s.app.Config.ConfigDir, deviceID).Value
	}
}

func (s *Server) deviceFrame(w http.ResponseWriter, r *http.Request, deviceID string) {
	if r.Method != http.MethodPost {
		writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	device, err := s.app.Store.Device(r.Context(), deviceID)
	if err != nil {
		writeError(w, err, http.StatusNotFound)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Stream   string `json:"stream"`
		Save     bool   `json:"save"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	candidates, err := s.frameCredentialCandidates(r.Context(), device, req.Username, req.Password, req.Stream)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	settings, _ := s.app.Store.Settings(r.Context())
	captureHost := strings.TrimSpace(settings["capture_ssh_host"])
	if captureHost == "" {
		captureHost = strings.TrimSpace(s.app.Config.CaptureSSHHost)
	}
	endpoint, err := s.app.StreamEndpointForDevice(ctx, device)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	var image []byte
	var rawURL string
	var used credentialCandidate
	var failures []string
	for _, candidate := range candidates {
		rawURL = cameraRTSPURL(candidate.Username, candidate.Password, endpoint.Host, endpoint.Port, candidate.Stream)
		image, err = captureFrameFunc(ctx, rawURL, captureHost)
		if err == nil {
			used = candidate
			break
		}
		failures = append(failures, candidate.Source+": "+frameCaptureError(ctx, err).Error())
	}
	if len(image) == 0 {
		if len(failures) > 0 {
			writeError(w, errors.New(strings.Join(failures, " · ")), http.StatusBadGateway)
			return
		}
		writeError(w, errors.New("username and password are required for frame capture"), http.StatusBadRequest)
		return
	}
	sum := sha256.Sum256(image)
	imagePath := ""
	if req.Save {
		if err := os.MkdirAll(s.app.Config.ReferenceImageDir(), 0o750); err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
		imagePath = filepath.Join(s.app.Config.ReferenceImageDir(), device.ID+".jpg")
		if err := os.WriteFile(imagePath, image, 0o600); err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
		_ = s.app.Store.PutSettings(r.Context(), map[string]string{"camera.reference_image." + device.ID: imagePath, "camera.reference_hash." + device.ID: hex.EncodeToString(sum[:])})
	}
	if used.IdentityID != "" {
		_ = s.rememberIdentityForDevice(r.Context(), device.ID, used)
	}
	writeJSON(w, map[string]any{
		"content_type":      "image/jpeg",
		"image_base64":      base64.StdEncoding.EncodeToString(image),
		"sha256":            hex.EncodeToString(sum[:]),
		"url_redacted":      redaction.URL(rawURL),
		"saved_path":        imagePath,
		"credential_source": used.Source,
		"identity_id":       used.IdentityID,
	}, http.StatusOK)
}

var captureFrameFunc = captureFrame

func captureFrame(ctx context.Context, rawURL, sshHost string) ([]byte, error) {
	args := []string{"-hide_banner", "-loglevel", "error", "-rtsp_transport", "tcp", "-i", rawURL, "-frames:v", "1", "-f", "image2", "-vcodec", "mjpeg", "pipe:1"}
	if sshHost != "" {
		if _, err := exec.LookPath("ssh"); err != nil {
			return nil, errors.New("SSH ist nicht installiert; Remote-Capture kann nicht ausgeführt werden")
		}
		sshArgs := append([]string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=3", sshHost, "ffmpeg"}, args...)
		return exec.CommandContext(ctx, "ssh", sshArgs...).Output()
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("ffmpeg ist nicht installiert; Vorschaubild kann nicht erzeugt werden")
	}
	return exec.CommandContext(ctx, ffmpeg, args...).Output()
}

func (s *Server) frameCredentialCandidates(ctx context.Context, device state.Device, username, password, stream string) ([]credentialCandidate, error) {
	settings, err := s.app.Store.Settings(ctx)
	if err != nil {
		return nil, err
	}
	stream = strings.TrimSpace(stream)
	if stream == "" {
		stream = settings["camera.credentials."+device.ID+".stream"]
	}
	if stream == "" {
		stream = "stream2"
	}
	username = strings.TrimSpace(username)
	// Passwords are used verbatim; only whitespace-only input is treated as
	// empty so saved passwords with surrounding spaces keep working.
	if strings.TrimSpace(password) == "" {
		password = secrets.LoadCamera(s.app.Config.ConfigDir, device.ID).Value
	}
	var candidates []credentialCandidate
	if username != "" && password != "" {
		candidates = append(candidates, credentialCandidate{Source: "kamera", Username: username, Password: password, Stream: stream})
	}
	if shouldTryCredentialIdentities(device, settings) {
		for _, identity := range s.credentialIdentitiesFromSettings(settings) {
			secret := secrets.LoadIdentity(s.app.Config.ConfigDir, identity.ID)
			if identity.Username == "" || secret.Value == "" {
				continue
			}
			candidate := credentialCandidate{Source: "identität " + identity.Name, IdentityID: identity.ID, Username: identity.Username, Password: secret.Value, Stream: stream}
			if !sameCredentialCandidate(candidates, candidate) {
				candidates = append(candidates, candidate)
			}
		}
	}
	if len(candidates) == 0 {
		return nil, errors.New("username and password are required for frame capture")
	}
	if len(candidates) > 8 {
		candidates = candidates[:8]
	}
	return candidates, nil
}

func sameCredentialCandidate(candidates []credentialCandidate, candidate credentialCandidate) bool {
	for _, existing := range candidates {
		if existing.Username == candidate.Username && existing.Password == candidate.Password && existing.Stream == candidate.Stream {
			return true
		}
	}
	return false
}

func shouldTryCredentialIdentities(device state.Device, settings map[string]string) bool {
	if settings["camera.disable_identity_probe"] == "true" {
		return false
	}
	if device.MACAddress != "" || device.ONVIFEndpointRef != "" || device.SerialNumber != "" || device.HardwareID != "" || device.Hostname != "" {
		return true
	}
	var raw map[string]any
	if len(device.RawJSON) > 0 && json.Unmarshal(device.RawJSON, &raw) == nil {
		if raw["manual"] == true || raw["rtsp_port_open"] == true || raw["onvif_port_open"] == true {
			return true
		}
	}
	return false
}

func (s *Server) rememberIdentityForDevice(ctx context.Context, deviceID string, candidate credentialCandidate) error {
	if candidate.IdentityID == "" {
		return nil
	}
	values := map[string]string{
		"camera.credentials." + deviceID + ".username":    candidate.Username,
		"camera.credentials." + deviceID + ".stream":      candidate.Stream,
		"camera.credentials." + deviceID + ".identity_id": candidate.IdentityID,
	}
	if err := s.app.Store.PutSettings(ctx, values); err != nil {
		return err
	}
	_, err := secrets.SaveCamera(s.app.Config.ConfigDir, deviceID, candidate.Password)
	return err
}

func (s *Server) getCredentialIdentities(w http.ResponseWriter, r *http.Request) {
	settings, err := s.app.Store.Settings(r.Context())
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, s.credentialIdentitiesFromSettings(settings), http.StatusOK)
}

func (s *Server) saveCredentialIdentity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID                 string `json:"id"`
		Name               string `json:"name"`
		Username           string `json:"username"`
		Password           string `json:"password"`
		CopyPasswordFromID string `json:"copy_password_from_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	req.Name = strings.TrimSpace(req.Name)
	req.Username = strings.TrimSpace(req.Username)
	req.CopyPasswordFromID = strings.TrimSpace(req.CopyPasswordFromID)
	if req.Name == "" {
		writeError(w, errors.New("name is required"), http.StatusBadRequest)
		return
	}
	if req.Username == "" {
		writeError(w, errors.New("username is required"), http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		req.ID = newCredentialIdentityID(req.Name)
	}
	settings, err := s.app.Store.Settings(r.Context())
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	ids := appendCredentialIdentityID(credentialIdentityIDs(settings), req.ID)
	values := map[string]string{
		credentialIdentityIDsKey:                  strings.Join(ids, ","),
		credentialIdentityKey(req.ID, "name"):     req.Name,
		credentialIdentityKey(req.ID, "username"): req.Username,
	}
	if err := s.app.Store.PutSettings(r.Context(), values); err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	source := ""
	if strings.TrimSpace(req.Password) != "" {
		source, err = secrets.SaveIdentity(s.app.Config.ConfigDir, req.ID, req.Password)
		if err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
	} else if req.CopyPasswordFromID != "" && req.CopyPasswordFromID != req.ID {
		secret := secrets.LoadIdentity(s.app.Config.ConfigDir, req.CopyPasswordFromID)
		if secret.Value != "" {
			source, err = secrets.SaveIdentity(s.app.Config.ConfigDir, req.ID, secret.Value)
			if err != nil {
				writeError(w, err, http.StatusInternalServerError)
				return
			}
		}
	}
	_ = s.app.Store.AddEvent(r.Context(), "info", "credentials.identity.updated", "Kamera-Identität wurde gespeichert", map[string]string{"identity_id": req.ID, "password_source": source})
	settings, _ = s.app.Store.Settings(r.Context())
	for _, identity := range s.credentialIdentitiesFromSettings(settings) {
		if identity.ID == req.ID {
			writeJSON(w, identity, http.StatusOK)
			return
		}
	}
	writeJSON(w, credentialIdentity{ID: req.ID, Name: req.Name, Username: req.Username}, http.StatusOK)
}

func (s *Server) deleteCredentialIdentity(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, errors.New("identity id is required"), http.StatusBadRequest)
		return
	}
	settings, err := s.app.Store.Settings(r.Context())
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	ids := removeCredentialIdentityID(credentialIdentityIDs(settings), id)
	if err := s.app.Store.PutSettings(r.Context(), map[string]string{credentialIdentityIDsKey: strings.Join(ids, ",")}); err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	// Remove the stored secret as well so credentials never outlive their
	// identity entry.
	secrets.DeleteIdentity(s.app.Config.ConfigDir, id)
	_ = s.app.Store.AddEvent(r.Context(), "info", "credentials.identity.deleted", "Kamera-Identität wurde entfernt", map[string]string{"identity_id": id})
	writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (s *Server) credentialIdentitiesFromSettings(settings map[string]string) []credentialIdentity {
	ids := credentialIdentityIDs(settings)
	identities := make([]credentialIdentity, 0, len(ids))
	for _, id := range ids {
		identity := credentialIdentity{
			ID:       id,
			Name:     strings.TrimSpace(settings[credentialIdentityKey(id, "name")]),
			Username: strings.TrimSpace(settings[credentialIdentityKey(id, "username")]),
		}
		if identity.Name == "" {
			identity.Name = id
		}
		secret := secrets.LoadIdentity(s.app.Config.ConfigDir, id)
		identity.PasswordSet = secret.Value != ""
		identity.PasswordSource = secret.Source
		identities = append(identities, identity)
	}
	return identities
}

func credentialIdentityIDs(settings map[string]string) []string {
	raw := settings[credentialIdentityIDsKey]
	if raw == "" {
		return nil
	}
	seen := map[string]bool{}
	ids := []string{}
	for _, part := range strings.Split(raw, ",") {
		id := strings.TrimSpace(part)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func appendCredentialIdentityID(ids []string, id string) []string {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

func removeCredentialIdentityID(ids []string, id string) []string {
	out := ids[:0]
	for _, existing := range ids {
		if existing != id {
			out = append(out, existing)
		}
	}
	return out
}

func credentialIdentityKey(id, field string) string {
	return "camera.identity." + sanitizeCredentialIdentityID(id) + "." + field
}

func newCredentialIdentityID(name string) string {
	base := sanitizeCredentialIdentityID(strings.ToLower(strings.TrimSpace(name)))
	if base == "" {
		base = "identity"
	}
	return fmt.Sprintf("%s_%d", base, time.Now().UTC().UnixNano())
}

func sanitizeCredentialIdentityID(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func cameraRTSPURL(username, password, host, port, stream string) string {
	u := neturl.URL{
		Scheme: "rtsp",
		User:   neturl.UserPassword(username, password),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + strings.TrimLeft(stream, "/"),
	}
	return u.String()
}

func frameCaptureError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.New("Vorschaubild konnte nicht gezogen werden: Zeitlimit nach 8 Sekunden. Kamera antwortet zu langsam, Stream ist blockiert oder RTSP ist nicht freigegeben.")
	}
	message := "Vorschaubild konnte nicht gezogen werden. Prüfe Benutzername, Passwort, Stream und RTSP-Freigabe."
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		detail := redaction.Text(string(exitErr.Stderr))
		if detail != "" {
			message += " ffmpeg: " + truncate(detail, 360)
		}
	}
	return errors.New(message)
}

func truncate(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= max {
		return value
	}
	// Cut at a rune boundary so multi-byte characters stay intact.
	cut := value[:max]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "..."
}

func (s *Server) deviceReferenceImage(w http.ResponseWriter, r *http.Request, deviceID string) {
	settings, err := s.app.Store.Settings(r.Context())
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	imagePath := settings["camera.reference_image."+deviceID]
	if imagePath == "" {
		writeError(w, errors.New("reference image not found"), http.StatusNotFound)
		return
	}
	cleanPath := filepath.Clean(imagePath)
	referenceDir := filepath.Clean(s.app.Config.ReferenceImageDir())
	if cleanPath != filepath.Join(referenceDir, filepath.Base(cleanPath)) {
		writeError(w, errors.New("invalid reference image path"), http.StatusBadRequest)
		return
	}
	info, err := os.Stat(cleanPath)
	if err != nil || info.IsDir() {
		writeError(w, errors.New("reference image not found"), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=30")
	http.ServeFile(w, r, cleanPath)
}

func (s *Server) getDeviceCredentials(w http.ResponseWriter, r *http.Request, deviceID string) {
	settings, err := s.app.Store.Settings(r.Context())
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	secret := secrets.LoadCamera(s.app.Config.ConfigDir, deviceID)
	writeJSON(w, map[string]any{
		"username":        settings["camera.credentials."+deviceID+".username"],
		"stream":          settings["camera.credentials."+deviceID+".stream"],
		"password_set":    secret.Value != "",
		"password_source": secret.Source,
	}, http.StatusOK)
}

func (s *Server) setDeviceCredentials(w http.ResponseWriter, r *http.Request, deviceID string) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Stream   string `json:"stream"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	if req.Stream == "" {
		req.Stream = "stream2"
	}
	source := ""
	if strings.TrimSpace(req.Password) != "" {
		var err error
		// Persist the password first: a failure here must not leave a saved
		// username that silently pairs with no credentials.
		source, err = secrets.SaveCamera(s.app.Config.ConfigDir, deviceID, req.Password)
		if err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
	}
	values := map[string]string{
		"camera.credentials." + deviceID + ".username": strings.TrimSpace(req.Username),
		"camera.credentials." + deviceID + ".stream":   strings.TrimSpace(req.Stream),
	}
	if err := s.app.Store.PutSettings(r.Context(), values); err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	_ = s.app.Store.AddEvent(r.Context(), "info", "camera.credentials.updated", "Kamera-Zugangsdaten wurden gespeichert", map[string]string{"device_id": deviceID, "password_source": source})
	secret := secrets.LoadCamera(s.app.Config.ConfigDir, deviceID)
	writeJSON(w, map[string]any{
		"status":          "ok",
		"username":        values["camera.credentials."+deviceID+".username"],
		"stream":          values["camera.credentials."+deviceID+".stream"],
		"password_set":    secret.Value != "",
		"password_source": secret.Source,
	}, http.StatusOK)
}

func (s *Server) getSlots(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.app.Slots, http.StatusOK)
}

func (s *Server) getBindings(w http.ResponseWriter, r *http.Request) {
	bindings, err := s.app.Store.Bindings(r.Context())
	writeResult(w, bindings, err)
}

func (s *Server) postBinding(w http.ResponseWriter, r *http.Request) {
	var binding state.Binding
	if err := json.NewDecoder(r.Body).Decode(&binding); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	err := s.app.Assign(r.Context(), binding)
	writeResult(w, map[string]string{"status": "ok"}, err)
}

func (s *Server) deleteBinding(w http.ResponseWriter, r *http.Request) {
	slotID := strings.TrimPrefix(r.URL.Path, "/api/bindings/")
	err := s.app.RemoveBinding(r.Context(), slotID)
	writeResult(w, map[string]string{"status": "ok"}, err)
}

func (s *Server) replaceBinding(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/replace") {
		writeError(w, errors.New("not found"), http.StatusNotFound)
		return
	}
	slotID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/bindings/"), "/replace")
	var binding state.Binding
	if err := json.NewDecoder(r.Body).Decode(&binding); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	binding.SlotID = slotID
	err := s.app.Assign(r.Context(), binding)
	writeResult(w, map[string]string{"status": "ok"}, err)
}

func (s *Server) renderGo2RTC(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.RenderConfiguredGo2RTC(r.Context())
	writeResult(w, result, err)
}

func (s *Server) restartGo2RTC(w http.ResponseWriter, r *http.Request) {
	err := s.app.RestartStreams(r.Context())
	if err == nil {
		_ = s.app.Store.AddEvent(r.Context(), "info", "go2rtc.restarted", "go2rtc wurde neu gestartet", nil)
	}
	writeResult(w, map[string]string{"status": "ok"}, err)
}

func (s *Server) getRelayStatus(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.RelayStatuses(r.Context())
	writeResult(w, result, err)
}

func (s *Server) startRelay(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, errors.New("relay id is required"), http.StatusBadRequest)
		return
	}
	result, err := s.app.StartRelay(r.Context(), id)
	writeResult(w, result, err)
}

func (s *Server) stopRelay(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, errors.New("relay id is required"), http.StatusBadRequest)
		return
	}
	result, err := s.app.StopRelay(r.Context(), id)
	writeResult(w, result, err)
}

func (s *Server) restartRelay(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, errors.New("relay id is required"), http.StatusBadRequest)
		return
	}
	result, err := s.app.RestartRelay(r.Context(), id)
	writeResult(w, result, err)
}

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
	if parsed.Scheme != "https" && os.Getenv("CAMERA_APPLIANCE_ALLOW_INSECURE_UPDATE") != "1" {
		writeError(w, errors.New("update url muss https verwenden"), http.StatusBadRequest)
		return
	}
	digest := strings.TrimSpace(req.Digest)
	if !s.updateMu.TryLock() {
		writeError(w, errors.New("es läuft bereits ein Update"), http.StatusConflict)
		return
	}
	_ = s.app.Store.AddEvent(r.Context(), "info", "update.started", "Update wurde gestartet", map[string]string{"url": redaction.Text(updateURL)})
	go func() {
		defer s.updateMu.Unlock()
		s.runUpdate(updateURL, digest)
	}()
	writeJSON(w, map[string]string{"status": "started", "url": updateURL}, http.StatusAccepted)
}

func (s *Server) runUpdate(updateURL, digest string) {
	s.runApply(func(ctx context.Context) (updater.Result, error) {
		return updater.Apply(ctx, updater.Options{
			Config:       s.app.Config,
			URL:          updateURL,
			Digest:       digest,
			InstallDir:   s.app.Config.InstallDir,
			AutoRollback: true,
			Restart:      updater.StackRestart(s.app.Config),
			Healthcheck:  updater.HTTPHealthcheck(s.app.Config),
		})
	}, updateURL)
}

// runApply executes an update job and records the outcome. It is shared by the
// URL-based and archive-based (download-then-install) flows.
func (s *Server) runApply(build func(ctx context.Context) (updater.Result, error), label string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	result, err := build(ctx)
	if err != nil {
		_ = s.app.Store.AddEvent(context.Background(), "error", "update.failed", err.Error(), map[string]string{"url": label})
		s.updates.setPhase(updateFailed, err.Error())
		s.updates.cleanupCachedArchive()
		return
	}
	_ = s.app.Store.AddEvent(context.Background(), "info", "update.completed", "Update installiert", map[string]string{
		"version": result.NewVersion.Version,
		"commit":  result.NewVersion.Commit,
		"backup":  result.BackupPath,
	})
	s.updates.mu.Lock()
	s.updates.st.Phase = updateIdle
	s.updates.st.CurrentVersion = result.NewVersion.Version
	s.updates.st.Error = ""
	s.updates.mu.Unlock()
	s.updates.cleanupCachedArchive()
}

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

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	dist := s.app.Config.FrontendDist
	path := filepath.Join(dist, filepath.Clean(r.URL.Path))
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	index := filepath.Join(dist, "index.html")
	if _, err := os.Stat(index); err == nil {
		http.ServeFile(w, r, index)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<html><body><h1>camera-appliance</h1><p>Frontend wurde noch nicht gebaut. Bitte npm run build im frontend-Verzeichnis ausführen.</p></body></html>`))
}

func (s *Server) getGo2RTCAsset(w http.ResponseWriter, r *http.Request) {
	asset := strings.TrimSpace(r.PathValue("asset"))
	if asset != "video-stream.js" && asset != "video-rtc.js" {
		writeError(w, errors.New("go2rtc asset not found"), http.StatusNotFound)
		return
	}
	base, err := neturl.Parse(strings.TrimSpace(s.app.Config.Go2RTCURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		writeError(w, errors.New("go2rtc url is invalid"), http.StatusBadGateway)
		return
	}
	base.User = nil
	base.Path = strings.TrimRight(base.Path, "/") + "/" + asset
	base.RawQuery = ""
	base.Fragment = ""

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		writeError(w, err, http.StatusBadGateway)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, err, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		writeError(w, fmt.Errorf("go2rtc asset request failed: %s", resp.Status), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, resp.Body)
}

func (s *Server) proxyGo2RTCWebSocket(w http.ResponseWriter, r *http.Request) {
	// Browsers always send Origin on WebSocket handshakes; a missing or
	// cross-site Origin means the handshake did not come from this appliance's
	// own pages, so refuse it before any video frames can be read.
	if !originMatchesRequest(r.Header.Get("Origin"), r.Host) {
		writeError(w, errors.New("websocket-origin abgelehnt"), http.StatusForbidden)
		return
	}
	target, err := neturl.Parse(strings.TrimSpace(s.app.Config.Go2RTCURL))
	if err != nil || target.Scheme == "" || target.Host == "" {
		writeError(w, errors.New("go2rtc url is invalid"), http.StatusBadGateway)
		return
	}
	target.User = nil
	basePath := strings.TrimRight(target.Path, "/")
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = basePath + "/api/ws"
		req.URL.RawQuery = r.URL.RawQuery
		req.Host = target.Host
		req.Header.Set("Origin", target.Scheme+"://"+target.Host)
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		writeError(w, err, http.StatusBadGateway)
	}
	proxy.ServeHTTP(w, r)
}

const sessionCookieName = "camera_appliance_session"

type authContextKey struct{}

type requestAuthInfo struct {
	Role             string
	ExpiresAt        *time.Time
	LocalAdminBypass bool
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := s.requestAuthInfo(r)
		ctx := context.WithValue(r.Context(), authContextKey{}, info)
		r = r.WithContext(ctx)

		status, err := s.app.AuthStatus(r.Context(), info.Role, info.ExpiresAt, info.LocalAdminBypass)
		if err != nil {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				writeError(w, err, http.StatusInternalServerError)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if !status.Enabled {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/go2rtc/") {
			if isPublicAuthAPI(r) {
				next.ServeHTTP(w, r)
				return
			}
			requiredRole := requiredAPIRole(r)
			if requiredRole == authn.RoleViewer && status.ViewerPublic {
				next.ServeHTTP(w, r)
				return
			}
			if roleAllowed(info.Role, requiredRole) {
				next.ServeHTTP(w, r)
				return
			}
			writeError(w, errors.New("login required"), unauthorizedStatus(info.Role, requiredRole))
			return
		}
		if shouldRedirectStaticToLogin(r.URL.Path, status, info) {
			nextParam := r.URL.RequestURI()
			http.Redirect(w, r, "/login?next="+neturl.QueryEscape(nextParam), http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestAuthInfo(r *http.Request) requestAuthInfo {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		session, sessionErr := s.app.AuthSession(r.Context(), cookie.Value)
		if sessionErr == nil && authn.IsRole(session.Role) {
			expiresAt := session.ExpiresAt
			return requestAuthInfo{Role: session.Role, ExpiresAt: &expiresAt}
		}
	}
	status, err := s.app.AuthStatus(r.Context(), "", nil, false)
	if err == nil && !status.Enabled {
		return requestAuthInfo{Role: authn.RoleAdmin}
	}
	if err == nil && status.LocalAdminBypass && isLoopbackRemote(r.RemoteAddr) && isLoopbackHost(r.Host) {
		return requestAuthInfo{Role: authn.RoleAdmin, LocalAdminBypass: true}
	}
	return requestAuthInfo{}
}

// isLoopbackHost reports whether the request's Host header points at the local
// machine. This blocks DNS-rebinding attempts: an attacker page can make
// requests that physically originate from loopback, but their Host header will
// name the attacker's domain instead of a loopback address.
func isLoopbackHost(host string) bool {
	if host == "" {
		return false
	}
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	hostname = strings.Trim(hostname, "[]")
	if ip := net.ParseIP(hostname); ip != nil {
		return ip.IsLoopback()
	}
	return strings.EqualFold(hostname, "localhost")
}

func authInfoFromContext(ctx context.Context) requestAuthInfo {
	info, _ := ctx.Value(authContextKey{}).(requestAuthInfo)
	return info
}

func isPublicAuthAPI(r *http.Request) bool {
	switch r.URL.Path {
	case "/api/auth/status", "/api/health":
		return r.Method == http.MethodGet
	case "/api/auth/login", "/api/auth/logout", "/api/auth/password":
		return r.Method == http.MethodPost
	default:
		return false
	}
}

func requiredAPIRole(r *http.Request) string {
	if r.Method == http.MethodGet && (r.URL.Path == "/api/viewer" || strings.HasPrefix(r.URL.Path, "/go2rtc/")) {
		return authn.RoleViewer
	}
	return authn.RoleAdmin
}

func roleAllowed(role, requiredRole string) bool {
	if requiredRole == authn.RoleViewer {
		return role == authn.RoleViewer || role == authn.RoleAdmin
	}
	return role == authn.RoleAdmin
}

func unauthorizedStatus(role, requiredRole string) int {
	if role != "" && !roleAllowed(role, requiredRole) {
		return http.StatusForbidden
	}
	return http.StatusUnauthorized
}

func shouldRedirectStaticToLogin(path string, status app.AuthStatus, info requestAuthInfo) bool {
	if path == "/login" || isStaticAssetPath(path) {
		return false
	}
	if isAdminUIPath(path) {
		return info.Role != authn.RoleAdmin
	}
	if path == "/" && !status.ViewerPublic {
		return info.Role != authn.RoleAdmin && info.Role != authn.RoleViewer
	}
	return false
}

func isStaticAssetPath(path string) bool {
	if strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/fonts/") {
		return true
	}
	return filepath.Ext(path) != ""
}

func isAdminUIPath(path string) bool {
	for _, prefix := range []string{
		"/einrichtung",
		"/uebersicht",
		"/system",
		"/kamera",
		"/setup",
		"/overview",
		"/cameras",
		"/discovery",
		"/assign",
		"/bindings",
		"/devices",
		"/settings",
		"/events",
		"/backup",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func isLoopbackRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time, remember bool) {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	}
	if remember {
		cookie.Expires = expiresAt
		cookie.MaxAge = max(1, int(time.Until(expiresAt).Seconds()))
	}
	http.SetCookie(w, cookie)
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
}

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

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob:; media-src 'self' blob:; connect-src 'self' ws: wss:; "+
				"font-src 'self'; frame-ancestors 'self'; base-uri 'self'; form-action 'self'; object-src 'none'")
		next.ServeHTTP(w, r)
	})
}

// withOriginCheck rejects state-changing requests whose Origin header names a
// different site. Browsers attach Origin to cross-site requests even though
// CORS lets them through for simple requests, so this blocks CSRF regardless
// of cookie SameSite behaviour.
func withOriginCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && !originMatchesRequest(origin, r.Host) {
			writeError(w, errors.New("cross-origin request abgelehnt"), http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func originMatchesRequest(origin, host string) bool {
	parsed, err := neturl.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Host, host)
}

type loginLimiter struct {
	mu    sync.Mutex
	fails map[string]*loginFailures
}

type loginFailures struct {
	count    int
	windowAt time.Time
}

const (
	loginFailLimit  = 10
	loginFailWindow = time.Minute
)

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{fails: map[string]*loginFailures{}}
}

// Allow reports whether a login attempt from addr may proceed or the failure
// limit within the current window has been exhausted.
func (l *loginLimiter) Allow(addr string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := l.fails[clientKey(addr)]
	if f == nil || now.Sub(f.windowAt) >= loginFailWindow {
		return true
	}
	return f.count < loginFailLimit
}

func (l *loginLimiter) RecordFailure(addr string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := clientKey(addr)
	f := l.fails[key]
	if f == nil || now.Sub(f.windowAt) >= loginFailWindow {
		l.fails[key] = &loginFailures{count: 1, windowAt: now}
		return
	}
	f.count++
}

func clientKey(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
