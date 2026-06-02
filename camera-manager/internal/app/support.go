package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/version"
)

type SupportBundleResult struct {
	Path    string   `json:"path"`
	Files   []string `json:"files"`
	Warning string   `json:"warning"`
}

func (a *App) CreateSupportBundle(ctx context.Context, out string) (SupportBundleResult, error) {
	if out == "" {
		out = filepath.Join(a.Config.BackupDir(), "support-bundle-"+time.Now().UTC().Format("20060102-150405")+".tar.gz")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		return SupportBundleResult{}, err
	}
	file, err := os.Create(out)
	if err != nil {
		return SupportBundleResult{}, err
	}
	defer file.Close()
	gw := gzip.NewWriter(file)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	var files []string
	add := func(name string, data []byte) error {
		if err := addSupportBundleFile(tw, name, data); err != nil {
			return err
		}
		files = append(files, name)
		return nil
	}

	status, statusErr := a.Status(ctx)
	if statusErr != nil {
		_ = add("status-error.txt", []byte(redaction.Text(statusErr.Error())+"\n"))
	} else if data, err := marshalSupportJSON(status); err == nil {
		_ = add("status.json", data)
	}
	viewer, viewerErr := a.Viewer(ctx)
	if viewerErr != nil {
		_ = add("viewer-error.txt", []byte(redaction.Text(viewerErr.Error())+"\n"))
	} else if data, err := marshalSupportJSON(viewer); err == nil {
		_ = add("viewer.json", data)
		_ = add("network.txt", []byte(networkReport(viewer)))
	}
	events, err := a.Store.Events(ctx, 100)
	if err == nil {
		if data, marshalErr := marshalSupportJSON(events); marshalErr == nil {
			_ = add("events.json", data)
		}
	}
	settings, err := a.Store.Settings(ctx)
	if err == nil {
		if data, marshalErr := marshalSupportJSON(redactedSettings(settings)); marshalErr == nil {
			_ = add("settings.redacted.json", data)
		}
	}
	_ = add("version.json", mustSupportJSON(version.Current()))
	_ = add("docker.txt", []byte(a.dockerReport(ctx)))
	_ = add("go2rtc-streams.redacted.json", []byte(a.go2rtcStreamsReport(ctx)))
	if data, err := os.ReadFile(a.Config.Go2RTCConfigPath()); err == nil {
		_ = add("go2rtc.yaml.redacted", []byte(redaction.Text(string(data))+"\n"))
	}

	if len(files) == 0 {
		return SupportBundleResult{}, errors.New("support bundle did not include any files")
	}
	_ = a.Store.AddEvent(ctx, "info", "support.bundle.created", "Support-Bundle erstellt", map[string]string{"path": out})
	return SupportBundleResult{
		Path:    out,
		Files:   files,
		Warning: "Support-Bundle ist redacted, enthält aber Netzwerk- und Systemdiagnose. Nur an vertraute Empfänger weitergeben.",
	}, nil
}

func addSupportBundleFile(tw *tar.Writer, name string, data []byte) error {
	name = strings.TrimLeft(filepath.Clean(name), string(filepath.Separator))
	if name == "." || strings.Contains(name, "..") {
		return fmt.Errorf("invalid support bundle path %q", name)
	}
	header := &tar.Header{
		Name:    name,
		Mode:    0o600,
		Size:    int64(len(data)),
		ModTime: time.Now().UTC(),
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err := io.Copy(tw, bytes.NewReader(data))
	return err
}

func marshalSupportJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(redaction.Text(string(data))), '\n'), nil
}

func mustSupportJSON(value any) []byte {
	data, _ := marshalSupportJSON(value)
	return data
}

func redactedSettings(settings map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range settings {
		if isSensitiveSettingKey(key) {
			out[key] = "******"
			continue
		}
		out[key] = redaction.Text(value)
	}
	return out
}

func isSensitiveSettingKey(key string) bool {
	lower := strings.ToLower(key)
	for _, part := range []string{"password", "secret", "token"} {
		if strings.Contains(lower, part) {
			return true
		}
	}
	return false
}

func networkReport(viewer Viewer) string {
	var out strings.Builder
	fmt.Fprintf(&out, "checked_at: %s\n", viewer.CheckedAt.Format(time.RFC3339))
	for _, slot := range viewer.Slots {
		fmt.Fprintf(&out, "\n[%s] %s state=%s\n", slot.Alias, slot.Label, slot.State)
		if slot.Device != nil {
			fmt.Fprintf(&out, "device=%s ip=%s\n", slot.Device.ID, slot.Device.LastIP)
		}
		if slot.Path != nil {
			fmt.Fprintf(&out, "selected=%s kind=%s host=%s port=%s state=%s\n", slot.Path.ID, slot.Path.Kind, slot.Path.Host, slot.Path.Port, slot.Path.State)
		}
		for _, path := range slot.Paths {
			fmt.Fprintf(&out, "path=%s kind=%s host=%s port=%s state=%s message=%s\n", path.ID, path.Kind, path.Host, path.Port, path.State, path.Message)
		}
	}
	return out.String()
}

func (a *App) dockerReport(ctx context.Context) string {
	parts := []string{
		"$ docker ps --format table",
		commandText(ctx, "docker", "ps", "--format", "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}"),
		"",
		"$ docker compose ps",
		commandText(ctx, "docker", "compose", "-f", a.Config.ComposeFile, "ps"),
	}
	return redaction.Text(strings.Join(parts, "\n")) + "\n"
}

func (a *App) go2rtcStreamsReport(ctx context.Context) string {
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	url := strings.TrimRight(a.Config.Go2RTCURL, "/") + "/api/streams"
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return redaction.Text(err.Error()) + "\n"
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return redaction.Text(err.Error()) + "\n"
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return redaction.Text(err.Error()) + "\n"
	}
	return redaction.Text(string(body)) + "\n"
}

func commandText(ctx context.Context, name string, args ...string) string {
	cmdCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, name, args...)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil {
		text += "\nerror: " + err.Error()
	}
	return redaction.Text(strings.TrimSpace(text))
}
