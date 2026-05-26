package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/app"
	"camera-appliance/camera-manager/internal/backup"
	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/secrets"
	"camera-appliance/camera-manager/internal/state"
	"camera-appliance/camera-manager/internal/system"
)

type Server struct {
	app *app.App
	mux *http.ServeMux
}

func New(a *app.App) *Server {
	s := &Server{app: a, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return withJSONHeaders(s.mux)
}

func (s *Server) routes() {
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
	s.mux.HandleFunc("POST /api/system/restart-stack", s.restartStack)
	s.mux.HandleFunc("GET /api/settings", s.getSettings)
	s.mux.HandleFunc("PUT /api/settings", s.putSettings)
	s.mux.HandleFunc("POST /api/secrets/camera-password", s.setCameraPassword)
	s.mux.HandleFunc("GET /api/events", s.getEvents)
	s.mux.HandleFunc("POST /api/backup", s.createBackup)
	s.mux.HandleFunc("POST /api/restore", s.restoreBackup)
	s.mux.HandleFunc("/", s.static)
}

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.app.Status(r.Context())
	writeResult(w, status, err)
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
	runs, err := s.app.Store.ScanRuns(r.Context(), 100)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	for _, run := range runs {
		if run.ID == id {
			writeJSON(w, run, http.StatusOK)
			return
		}
	}
	writeError(w, errors.New("scan run not found"), http.StatusNotFound)
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
		values := map[string]string{
			"camera.credentials." + result.Device.ID + ".username": strings.TrimSpace(req.Username),
			"camera.credentials." + result.Device.ID + ".stream":   strings.TrimSpace(req.Stream),
		}
		if err := s.app.Store.PutSettings(r.Context(), values); err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
		if strings.TrimSpace(req.Password) != "" {
			if _, err := secrets.SaveCamera(s.app.Config.ConfigDir, result.Device.ID, req.Password); err != nil {
				writeError(w, err, http.StatusInternalServerError)
				return
			}
		}
	}
	writeJSON(w, result, http.StatusOK)
}

func (s *Server) getDevice(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/devices/")
	if strings.HasSuffix(path, "/preview") {
		writeJSON(w, map[string]string{"message": "Vorschau ist im MVP über AgentDVR/go2rtc manuell vorgesehen."}, http.StatusOK)
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
	rawURL := cameraRTSPURL(req.Username, req.Password, device.LastIP, req.Stream)
	ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
	defer cancel()
	conn, dialErr := (&net.Dialer{}).DialContext(ctx, "tcp", device.LastIP+":554")
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
	applySavedCredentials(r.Context(), s, deviceID, &req.Username, &req.Password, &req.Stream)
	if req.Stream == "" {
		req.Stream = "stream2"
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, errors.New("username and password are required for frame capture"), http.StatusBadRequest)
		return
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		writeError(w, errors.New("ffmpeg ist nicht installiert; Vorschaubild kann nicht erzeugt werden"), http.StatusServiceUnavailable)
		return
	}
	rawURL := cameraRTSPURL(req.Username, req.Password, device.LastIP, req.Stream)
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-rtsp_transport", "tcp", "-i", rawURL, "-frames:v", "1", "-f", "image2", "-vcodec", "mjpeg", "pipe:1")
	image, err := cmd.Output()
	if err != nil {
		writeError(w, frameCaptureError(ctx, err), http.StatusBadGateway)
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
	writeJSON(w, map[string]any{
		"content_type": "image/jpeg",
		"image_base64": base64.StdEncoding.EncodeToString(image),
		"sha256":       hex.EncodeToString(sum[:]),
		"url_redacted": redaction.URL(rawURL),
		"saved_path":   imagePath,
	}, http.StatusOK)
}

func cameraRTSPURL(username, password, host, stream string) string {
	u := neturl.URL{
		Scheme: "rtsp",
		User:   neturl.UserPassword(username, password),
		Host:   net.JoinHostPort(host, "554"),
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
	return value[:max] + "..."
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
	values := map[string]string{
		"camera.credentials." + deviceID + ".username": strings.TrimSpace(req.Username),
		"camera.credentials." + deviceID + ".stream":   strings.TrimSpace(req.Stream),
	}
	if err := s.app.Store.PutSettings(r.Context(), values); err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	source := ""
	if strings.TrimSpace(req.Password) != "" {
		var err error
		source, err = secrets.SaveCamera(s.app.Config.ConfigDir, deviceID, req.Password)
		if err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
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
	result, err := s.app.RenderGo2RTC(r.Context())
	writeResult(w, result, err)
}

func (s *Server) restartGo2RTC(w http.ResponseWriter, r *http.Request) {
	err := system.RestartGo2RTC(r.Context(), s.app.Config)
	if err == nil {
		_ = s.app.Store.AddEvent(r.Context(), "info", "go2rtc.restarted", "go2rtc wurde neu gestartet", nil)
	}
	writeResult(w, map[string]string{"status": "ok"}, err)
}

func (s *Server) restartStack(w http.ResponseWriter, r *http.Request) {
	err := system.RestartStack(r.Context(), s.app.Config)
	writeResult(w, map[string]string{"status": "ok"}, err)
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.app.Store.Settings(r.Context())
	if settings == nil {
		settings = map[string]string{}
	}
	settings["agentdvr_url"] = s.app.Config.AgentDVRURL
	settings["go2rtc_url"] = s.app.Config.Go2RTCURL
	settings["bind_addr"] = s.app.Config.BindAddr
	settings["camera_password_set"] = fmt.Sprintf("%t", s.app.Config.TapoPassword != "")
	settings["camera_password_source"] = s.app.Config.TapoPasswordSource
	writeResult(w, settings, err)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var settings map[string]string
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	delete(settings, "camera_password")
	err := s.app.Store.PutSettings(r.Context(), settings)
	writeResult(w, map[string]string{"status": "ok"}, err)
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
	s.app.Config.TapoPassword = req.Password
	s.app.Config.TapoPasswordSource = source
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

func writeResult(w http.ResponseWriter, value any, err error) {
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
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

func withJSONHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}
