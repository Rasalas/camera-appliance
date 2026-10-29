package app

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"

	"camera-appliance/camera-manager/internal/cameraaccess"
	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/secrets"
	"camera-appliance/camera-manager/internal/state"
)

var ErrInvalidCameraConfiguration = errors.New("Ungültige Kamera-Konfiguration.")

type CameraEndpoint struct {
	Host       string `json:"host"`
	Port       string `json:"port"`
	TargetHost string `json:"target_host,omitempty"`
	TargetPort string `json:"target_port,omitempty"`
}
type CameraConfiguration struct {
	Access     *cameraaccess.CredentialsInput `json:"access,omitempty"`
	Name       *string                        `json:"name,omitempty"`
	Visible    *bool                          `json:"visible,omitempty"`
	PathPolicy *string                        `json:"path_policy,omitempty"`
	// Display uses the existing suffixes, e.g. rotation and crop_width.
	Display        map[string]string         `json:"display,omitempty"`
	Endpoint       *CameraEndpoint           `json:"endpoint,omitempty"`
	RelayEndpoints map[string]CameraEndpoint `json:"relay_endpoints,omitempty"`
}
type CameraConfigurationResult struct {
	Persisted         bool     `json:"persisted"`
	PasswordPersisted bool     `json:"password_persisted"`
	Applied           bool     `json:"applied"`
	Restarted         bool     `json:"restarted"`
	Error             string   `json:"error,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`
}

// ConfigureCamera owns a complete save/apply sequence. Apply failure retains
// desired state and a pending marker so an identical retry attempts apply again.
func (a *App) ConfigureCamera(ctx context.Context, id string, req CameraConfiguration) (CameraConfigurationResult, error) {
	a.go2rtcMu.Lock()
	defer a.go2rtcMu.Unlock()
	result := CameraConfigurationResult{}
	if _, err := a.Store.Device(ctx, id); err != nil {
		return result, err
	}
	values := map[string]string{}
	streamChange := false
	invalid := func() (CameraConfigurationResult, error) { return result, ErrInvalidCameraConfiguration }
	if req.Name != nil {
		name, err := state.NormalizeDeviceName(*req.Name)
		if err != nil {
			return result, err
		}
		req.Name = &name
	}
	if req.PathPolicy != nil {
		switch *req.PathPolicy {
		case "auto", "direct_only", "relay_only", "prefer_direct", "prefer_relay":
		default:
			return invalid()
		}
		values["camera.path_policy."+id] = *req.PathPolicy
	}
	for key, value := range req.Display {
		valid := false
		switch key {
		case "rotation":
			valid = value == "0" || value == "90" || value == "180" || value == "270"
		case "mirror", "flip":
			valid = value == "true" || value == "false"
		case "fit_mode":
			valid = value == "contain" || value == "cover"
		case "crop_x", "crop_y", "crop_width", "crop_height":
			n, err := strconv.ParseFloat(value, 64)
			valid = err == nil && n >= 0 && n <= 100
		}
		if !valid {
			return invalid()
		}
		values["camera.display."+id+"."+key] = value
	}
	addEndpoint := func(prefix string, e CameraEndpoint) bool {
		if !validCameraEndpoint(e.Host, e.Port) {
			return false
		}
		values[prefix+"host"] = strings.TrimSpace(e.Host)
		values[prefix+"port"] = e.Port
		return true
	}

	if req.Endpoint != nil && !addEndpoint("camera.rtsp_endpoint."+id+".", *req.Endpoint) {
		return invalid()
	}
	for relay, e := range req.RelayEndpoints {
		if relay == "" || strings.ContainsAny(relay, "./\\ \n\r") {
			return invalid()
		}
		if !addEndpoint("camera.relay_endpoint."+id+"."+relay+".", e) {
			return invalid()
		}
		if !validCameraEndpoint(e.TargetHost, e.TargetPort) {
			return invalid()
		}
		prefix := "camera.relay_endpoint." + id + "." + relay + "."
		values[prefix+"target_host"] = strings.TrimSpace(e.TargetHost)
		values[prefix+"target_port"] = e.TargetPort

	}
	if req.Access != nil {
		stream := strings.TrimSpace(req.Access.Stream)
		if stream == "" {
			stream = "stream2"
		}
		if strings.ContainsAny(stream, "\n\r?#") {
			return invalid()
		}
		values["camera.credentials."+id+".username"] = strings.TrimSpace(req.Access.Username)
		values["camera.credentials."+id+".stream"] = stream
	}
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		return result, err
	}
	for key, value := range values {
		if !strings.HasPrefix(key, "camera.display.") && settings[key] != value {
			streamChange = true
		}
	}
	if req.Access != nil && strings.TrimSpace(req.Access.Password) != "" {
		if secrets.LoadCamera(a.Config.ConfigDir, id).Value != req.Access.Password {
			if _, err := secrets.SaveCamera(a.Config.ConfigDir, id, req.Access.Password); err != nil {
				return result, err
			}
			result.PasswordPersisted = true
			streamChange = true
			a.InvalidateCredentialPresence(id)
		}
	}
	pendingKey := "camera.configuration." + id + ".pending"
	if streamChange {
		values[pendingKey] = "true"
	}
	// Compare visibility inside the transaction; settings above are compared
	// separately to avoid treating display changes as stream changes.
	visibilityChanged, err := a.Store.ConfigureCamera(ctx, id, values, req.Name, req.Visible)
	if err != nil {
		return result, err
	}
	result.Persisted = true
	if req.Visible != nil {
		streamChange = streamChange || visibilityChanged
	}
	pending := settings[pendingKey] == "true"
	if !streamChange && !pending {
		result.Applied = true
		return result, nil
	}
	before, _ := os.ReadFile(a.Config.Go2RTCConfigPath())
	fail := func(err error) (CameraConfigurationResult, error) {
		result.Error = redaction.Text(err.Error())
		if e := a.Store.PutSettings(ctx, map[string]string{pendingKey: "true"}); e != nil {
			result.Error += "; " + redaction.Text(e.Error())
		}
		return result, nil
	}
	rendered, err := a.RenderGo2RTC(ctx)
	if err != nil {
		return fail(err)
	}
	result.Warnings = rendered.Warnings
	after, err := os.ReadFile(a.Config.Go2RTCConfigPath())
	if err != nil {
		return fail(err)
	}
	if pending || !bytes.Equal(before, after) {
		if err := a.restartGo2RTC(ctx); err != nil {
			return fail(err)
		}
		result.Restarted = true
	}
	if err := a.Store.PutSettings(ctx, map[string]string{pendingKey: "false"}); err != nil {
		return fail(err)
	}
	result.Applied = true
	return result, nil
}

func validCameraEndpoint(host, port string) bool {
	host = strings.TrimSpace(host)
	if strings.ContainsAny(host, "/@?#\n\r\t ") {
		return false
	}
	if strings.Contains(host, ":") && net.ParseIP(strings.Trim(host, "[]")) == nil {
		return false
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	return true
}
