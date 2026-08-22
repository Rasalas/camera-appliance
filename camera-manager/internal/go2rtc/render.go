package go2rtc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	neturl "net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/state"

	"gopkg.in/yaml.v3"
)

type RenderInput struct {
	Slots     []config.Slot
	Bindings  []state.Binding
	Password  string
	Passwords map[string]string
	Endpoints map[string]StreamEndpoint
	Output    string
}

type StreamEndpoint struct {
	Host string
	Port string
}

type RenderResult struct {
	Path            string   `json:"path"`
	RenderedStreams int      `json:"rendered_streams"`
	Warnings        []string `json:"warnings"`
	RedactedYAML    string   `json:"redacted_yaml"`
}

type document struct {
	Streams map[string][]string `yaml:"streams"`
}

func Render(ctx context.Context, input RenderInput) (RenderResult, error) {
	if input.Output == "" {
		return RenderResult{}, errors.New("output path is required")
	}
	slotMap := map[string]config.Slot{}
	for _, slot := range input.Slots {
		slotMap[slot.ID] = slot
	}
	sort.Slice(input.Bindings, func(i, j int) bool { return input.Bindings[i].SlotID < input.Bindings[j].SlotID })
	doc := document{Streams: map[string][]string{}}
	var warnings []string
	for _, binding := range input.Bindings {
		select {
		case <-ctx.Done():
			return RenderResult{}, ctx.Err()
		default:
		}
		if !binding.Enabled {
			continue
		}
		slot, ok := slotMap[binding.SlotID]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s ist kein bekannter Platz", binding.SlotID))
			continue
		}
		if binding.Device == nil || binding.Device.LastIP == "" {
			warnings = append(warnings, fmt.Sprintf("%s (%s) hat keine aktuelle IP-Adresse", slot.ID, displayLabel(slot, binding)))
			continue
		}
		if strings.TrimSpace(binding.Username) == "" {
			warnings = append(warnings, fmt.Sprintf("%s (%s) hat keinen Benutzernamen", slot.ID, displayLabel(slot, binding)))
			continue
		}
		password := input.Password
		if input.Passwords != nil && binding.DeviceID != "" && input.Passwords[binding.DeviceID] != "" {
			password = input.Passwords[binding.DeviceID]
		}
		if password == "" {
			warnings = append(warnings, fmt.Sprintf("%s (%s) wurde übersprungen: Kamera-Passwort fehlt", slot.ID, displayLabel(slot, binding)))
			continue
		}
		endpoint := input.Endpoints[binding.DeviceID]
		if strings.TrimSpace(endpoint.Host) == "" {
			endpoint.Host = binding.Device.LastIP
		}
		if strings.TrimSpace(endpoint.Port) == "" {
			endpoint.Port = "554"
		}
		stream := binding.StreamName
		if stream == "" {
			stream = slot.DefaultStream
		}
		doc.Streams[slot.ID] = []string{cameraRTSPURL(binding.Username, password, endpoint.Host, endpoint.Port, stream)}
		for _, profile := range []string{"stream1", "stream2"} {
			doc.Streams[profileAlias(slot.ID, profile)] = []string{cameraRTSPURL(binding.Username, password, endpoint.Host, endpoint.Port, profile)}
		}
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return RenderResult{}, err
	}
	var validate document
	if err := yaml.Unmarshal(data, &validate); err != nil {
		return RenderResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(input.Output), 0o750); err != nil {
		return RenderResult{}, err
	}
	// Unique temp name per render: concurrent renders must not clobber each
	// other's file before the atomic rename publishes it.
	tmp, err := os.CreateTemp(filepath.Dir(input.Output), filepath.Base(input.Output)+".tmp-*")
	if err != nil {
		return RenderResult{}, err
	}
	tmpPath := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		_ = os.Remove(tmpPath)
		return RenderResult{}, writeErr
	}
	if err := os.Rename(tmpPath, input.Output); err != nil {
		_ = os.Remove(tmpPath)
		return RenderResult{}, err
	}
	redacted := redactYAML(data)
	return RenderResult{Path: input.Output, RenderedStreams: len(doc.Streams), Warnings: warnings, RedactedYAML: redacted}, nil
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

func profileAlias(slotID, stream string) string {
	return slotID + "_" + strings.Trim(strings.TrimSpace(stream), "/")
}

func displayLabel(slot config.Slot, binding state.Binding) string {
	if binding.Label != "" {
		return binding.Label
	}
	return slot.Label
}

func redactYAML(data []byte) string {
	lines := strings.Split(string(bytes.TrimSpace(data)), "\n")
	for i, line := range lines {
		lines[i] = redaction.Text(line)
	}
	return strings.Join(lines, "\n") + "\n"
}
