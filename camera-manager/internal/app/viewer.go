package app

import (
	"context"
	"net"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/secrets"
	"camera-appliance/camera-manager/internal/state"
	"camera-appliance/camera-manager/internal/system"

	"gopkg.in/yaml.v3"
)

const (
	ViewerStateUnassigned        = "unassigned"
	ViewerStateConnecting        = "connecting"
	ViewerStateOnline            = "online"
	ViewerStateOffline           = "offline"
	ViewerStateCredentialsFailed = "credentials_failed"
	ViewerStateStreamUnavailable = "stream_unavailable"
)

type Viewer struct {
	CheckedAt       time.Time            `json:"checked_at"`
	Go2RTC          system.ServiceStatus `json:"go2rtc"`
	GeneratedConfig string               `json:"generated_config,omitempty"`
	StreamCount     int                  `json:"stream_count"`
	Layout          ViewerLayout         `json:"layout"`
	Slots           []ViewerSlot         `json:"slots"`
}

type ViewerSlot struct {
	Slot        config.Slot        `json:"slot"`
	Alias       string             `json:"alias"`
	Label       string             `json:"label"`
	State       string             `json:"state"`
	Message     string             `json:"message"`
	Binding     *state.Binding     `json:"binding,omitempty"`
	Device      *state.Device      `json:"device,omitempty"`
	Playback    *ViewerPlayback    `json:"playback,omitempty"`
	Path        *StreamPath        `json:"path,omitempty"`
	Paths       []StreamPath       `json:"paths,omitempty"`
	Display     CameraDisplay      `json:"display"`
	Diagnostics []ViewerDiagnostic `json:"diagnostics,omitempty"`
}

type ViewerPlayback struct {
	PageURL string `json:"page_url"`
}

type ViewerDiagnostic struct {
	Key     string `json:"key"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type go2rtcConfigDocument struct {
	Streams map[string]any `yaml:"streams"`
}

func (a *App) Viewer(ctx context.Context) (Viewer, error) {
	bindings, err := a.Store.Bindings(ctx)
	if err != nil {
		return Viewer{}, err
	}
	bindings = attachSlots(bindings, a.Slots)
	settings, _ := a.Store.Settings(ctx)
	go2rtcStatus := system.Check(ctx, a.Config).Go2RTC
	aliases, generatedPath := a.generatedGo2RTCAliases()

	viewer := Viewer{
		CheckedAt:       time.Now().UTC(),
		Go2RTC:          go2rtcStatus,
		GeneratedConfig: generatedPath,
		StreamCount:     len(aliases),
		Layout:          viewerLayoutFromSettings(settings, a.Slots),
		Slots:           make([]ViewerSlot, 0, len(a.Slots)),
	}
	bindingBySlot := map[string]state.Binding{}
	for _, binding := range bindings {
		bindingBySlot[binding.SlotID] = binding
	}
	for _, slot := range a.Slots {
		viewer.Slots = append(viewer.Slots, a.viewerSlot(ctx, slot, bindingBySlot[slot.ID], settings, aliases, go2rtcStatus))
	}
	return viewer, nil
}

func (a *App) viewerSlot(ctx context.Context, slot config.Slot, binding state.Binding, settings map[string]string, aliases map[string]bool, go2rtcStatus system.ServiceStatus) ViewerSlot {
	item := ViewerSlot{
		Slot:    slot,
		Alias:   slot.ID,
		Label:   slot.Label,
		State:   ViewerStateUnassigned,
		Message: "Kein Gerät zugeordnet.",
		Display: displayFromSettings(settings, binding),
		Diagnostics: []ViewerDiagnostic{
			{Key: "assignment", Status: "missing", Message: "Platz ist leer."},
		},
	}
	if binding.SlotID == "" {
		return item
	}

	localBinding := binding
	item.Binding = &localBinding
	item.Label = displaySlotLabel(slot, binding)
	if binding.Device != nil {
		device := *binding.Device
		item.Device = &device
	}
	if !binding.Enabled {
		item.State = ViewerStateOffline
		item.Message = "Zuordnung ist deaktiviert."
		item.Diagnostics = []ViewerDiagnostic{{Key: "assignment", Status: "failed", Message: "Zuordnung ist deaktiviert."}}
		return item
	}
	if binding.Device == nil || strings.TrimSpace(binding.Device.LastIP) == "" {
		item.State = ViewerStateOffline
		item.Message = "Kamera hat keine aktuelle IP-Adresse."
		item.Diagnostics = []ViewerDiagnostic{
			{Key: "assignment", Status: "ok", Message: "Gerät ist zugeordnet."},
			{Key: "network", Status: "failed", Message: "Keine aktuelle IP-Adresse vorhanden."},
		}
		return item
	}
	pathAssessment := a.assessStreamPaths(ctx, binding, settings)
	item.Paths = pathAssessment.Paths
	if pathAssessment.Selected != nil {
		selected := *pathAssessment.Selected
		item.Path = &selected
	}
	if pathAssessment.Selected == nil {
		item.State = ViewerStateOffline
		item.Message = "Kein RTSP-Pfad ist erreichbar."
		item.Diagnostics = []ViewerDiagnostic{
			{Key: "assignment", Status: "ok", Message: "Gerät ist zugeordnet."},
			{Key: "network", Status: "failed", Message: pathFailureSummary(pathAssessment.Paths)},
		}
		return item
	}

	username := strings.TrimSpace(binding.Username)
	if username == "" {
		username = strings.TrimSpace(settings["camera.credentials."+binding.DeviceID+".username"])
	}
	passwordSet := a.Config.TapoPassword != ""
	if binding.DeviceID != "" && secrets.LoadCamera(a.Config.ConfigDir, binding.DeviceID).Value != "" {
		passwordSet = true
	}
	if username == "" || !passwordSet {
		item.State = ViewerStateCredentialsFailed
		item.Message = "Kamera-Zugangsdaten fehlen."
		item.Diagnostics = []ViewerDiagnostic{
			{Key: "assignment", Status: "ok", Message: "Gerät ist zugeordnet."},
			{Key: "network", Status: "ok", Message: "Letzte IP: " + binding.Device.LastIP},
			{Key: "path", Status: "ok", Message: streamPathDiagnostic(*pathAssessment.Selected)},
			{Key: "credentials", Status: "failed", Message: credentialDiagnosticMessage(username, passwordSet)},
		}
		return item
	}

	if !aliases[slot.ID] {
		item.State = ViewerStateStreamUnavailable
		item.Message = "go2rtc-Alias ist noch nicht erzeugt."
		item.Diagnostics = []ViewerDiagnostic{
			{Key: "assignment", Status: "ok", Message: "Gerät ist zugeordnet."},
			{Key: "path", Status: "ok", Message: streamPathDiagnostic(*pathAssessment.Selected)},
			{Key: "credentials", Status: "ok", Message: "Zugangsdaten sind hinterlegt."},
			{Key: "go2rtc", Status: "failed", Message: "Alias " + slot.ID + " fehlt in der erzeugten go2rtc-Konfiguration."},
		}
		return item
	}
	if !go2rtcStatus.Online {
		item.State = ViewerStateStreamUnavailable
		item.Message = "go2rtc ist nicht erreichbar."
		item.Diagnostics = []ViewerDiagnostic{
			{Key: "assignment", Status: "ok", Message: "Gerät ist zugeordnet."},
			{Key: "path", Status: "ok", Message: streamPathDiagnostic(*pathAssessment.Selected)},
			{Key: "credentials", Status: "ok", Message: "Zugangsdaten sind hinterlegt."},
			{Key: "go2rtc", Status: "failed", Message: go2rtcStatus.Message},
		}
		return item
	}

	item.State = ViewerStateOnline
	item.Message = "Stream-Alias ist bereit."
	item.Playback = &ViewerPlayback{PageURL: go2rtcStreamPageURL(a.Config.Go2RTCURL, slot.ID)}
	item.Diagnostics = []ViewerDiagnostic{
		{Key: "assignment", Status: "ok", Message: "Gerät ist zugeordnet."},
		{Key: "network", Status: "ok", Message: "Letzte IP: " + binding.Device.LastIP},
		{Key: "path", Status: "ok", Message: streamPathDiagnostic(*pathAssessment.Selected)},
		{Key: "credentials", Status: "ok", Message: "Zugangsdaten sind hinterlegt."},
		{Key: "go2rtc", Status: "ok", Message: "Alias " + slot.ID + " ist konfiguriert."},
	}
	if item.Playback.PageURL == "" {
		item.State = ViewerStateStreamUnavailable
		item.Message = "go2rtc-Player-URL konnte nicht gebildet werden."
		item.Playback = nil
		item.Diagnostics = []ViewerDiagnostic{
			{Key: "assignment", Status: "ok", Message: "Gerät ist zugeordnet."},
			{Key: "path", Status: "ok", Message: streamPathDiagnostic(*pathAssessment.Selected)},
			{Key: "credentials", Status: "ok", Message: "Zugangsdaten sind hinterlegt."},
			{Key: "go2rtc", Status: "failed", Message: "go2rtc-URL ist ungültig."},
		}
	}
	return item
}

func pathFailureSummary(paths []StreamPath) string {
	if len(paths) == 0 {
		return "Kein direkter oder Relay-Pfad ist konfiguriert."
	}
	for _, path := range paths {
		if path.Message != "" {
			return path.Label + ": " + path.Message
		}
	}
	return "Alle RTSP-Pfade sind nicht erreichbar."
}

func streamPathDiagnostic(path StreamPath) string {
	if path.State != "ok" {
		if path.StabilityMessage != "" {
			return "Pfad " + path.Label + ": " + path.Message + " " + path.StabilityMessage
		}
		return "Pfad " + path.Label + ": " + path.Message
	}
	prefix := "Pfad " + path.Label + " ist erreichbar"
	if path.Kind == PathKindDirect {
		prefix += " (direkt " + path.Host + ":" + path.Port + ")."
	} else {
		prefix += " (Relay " + path.Host + ":" + path.Port + ")."
	}
	if path.StabilityMessage != "" {
		return prefix + " " + path.StabilityMessage
	}
	return prefix
}

func (a *App) probeRTSP(ctx context.Context, host, port string) error {
	if a.RTSPProbe != nil {
		return a.RTSPProbe(ctx, host, port)
	}
	timeout := a.Config.RequestTimeout
	if timeout <= 0 {
		timeout = 700 * time.Millisecond
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(probeCtx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

func probeHostForEndpoint(host string) string {
	if strings.EqualFold(strings.TrimSpace(host), "host.docker.internal") {
		return "127.0.0.1"
	}
	return host
}

func rtspProbeDiagnostic(port string, err error) string {
	if strings.TrimSpace(port) == "" {
		port = "554"
	}
	if err == nil {
		return "RTSP-Port " + port + " ist erreichbar."
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "refused") {
		return "RTSP-Port " + port + " lehnt Verbindungen ab."
	}
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline") {
		return "RTSP-Port " + port + " antwortet nicht."
	}
	return "RTSP-Port " + port + " ist nicht erreichbar."
}

func (a *App) generatedGo2RTCAliases() (map[string]bool, string) {
	configPath := a.Config.Go2RTCConfigPath()
	aliases := map[string]bool{}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return aliases, configPath
	}
	var doc go2rtcConfigDocument
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return aliases, configPath
	}
	for alias := range doc.Streams {
		aliases[alias] = true
	}
	return aliases, configPath
}

func displaySlotLabel(slot config.Slot, binding state.Binding) string {
	if strings.TrimSpace(binding.Label) != "" {
		return strings.TrimSpace(binding.Label)
	}
	return slot.Label
}

func credentialDiagnosticMessage(username string, passwordSet bool) string {
	switch {
	case username == "" && !passwordSet:
		return "Benutzername und Passwort fehlen."
	case username == "":
		return "Benutzername fehlt."
	case !passwordSet:
		return "Passwort fehlt."
	default:
		return "Zugangsdaten unvollständig."
	}
}

func go2rtcStreamPageURL(baseURL, alias string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.User = nil
	parsed.Path = path.Join(parsed.Path, "stream.html")
	query := parsed.Query()
	query.Set("src", alias)
	query.Set("mode", "webrtc,mse,mp4,mjpeg")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
