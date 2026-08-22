package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/config"
)

type Result struct {
	Path    string   `json:"path"`
	Files   []string `json:"files"`
	Warning string   `json:"warning"`
}

func Create(ctx context.Context, cfg config.Config, out string, includeSecrets bool) (Result, error) {
	if out == "" {
		out = filepath.Join(cfg.BackupDir(), "camera-appliance-"+time.Now().UTC().Format("20060102-150405")+".tar.gz")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		return Result{}, err
	}
	// Backups contain camera credentials from local.env and the generated
	// go2rtc config; never create them world-readable.
	file, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return Result{}, err
	}
	defer file.Close()
	gw := gzip.NewWriter(file)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	files := []struct {
		path string
		name string
	}{
		{cfg.DBPath(), "var/lib/camera-appliance/state.db"},
		{cfg.Go2RTCConfigPath(), "var/lib/camera-appliance/generated/go2rtc.yaml"},
		{filepath.Join(cfg.ConfigDir, "local.env"), "etc/camera-appliance/local.env"},
	}
	if includeSecrets {
		files = append(files, struct{ path, name string }{filepath.Join(cfg.ConfigDir, "secrets.env"), "etc/camera-appliance/secrets.env"})
	}
	var included []string
	for _, item := range files {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		default:
		}
		if err := addFile(tw, item.path, item.name); err == nil {
			included = append(included, item.name)
		}
	}
	warning := "Backup enthält Kamera-Zugangsdaten aus local.env und ist sensibel. Geschützt speichern."
	if includeSecrets {
		warning = "Backup enthält secrets.env mit allen Zugangsdaten und ist besonders sensibel. Geschützt speichern."
	}
	return Result{Path: out, Files: included, Warning: warning}, nil
}

func Restore(ctx context.Context, cfg config.Config, in string) (Result, error) {
	if in == "" {
		return Result{}, errors.New("input backup path is required")
	}
	file, err := os.Open(in)
	if err != nil {
		return Result{}, err
	}
	defer file.Close()
	gr, err := gzip.NewReader(file)
	if err != nil {
		return Result{}, err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	var restored []string
	for {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		default:
		}
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Result{}, err
		}
		target, ok := restoreTarget(cfg, header.Name)
		if !ok || header.FileInfo().IsDir() {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return Result{}, err
		}
		// Only whitelisted regular files are restored here; clamp permissions
		// so archive-supplied bits (setuid, group/world access) cannot survive.
		mode := header.FileInfo().Mode().Perm()
		if mode&0o400 == 0 {
			mode |= 0o400
		}
		mode &= 0o600
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			return Result{}, err
		}
		_, copyErr := io.Copy(out, tr)
		closeErr := out.Close()
		if copyErr != nil {
			return Result{}, copyErr
		}
		if closeErr != nil {
			return Result{}, closeErr
		}
		restored = append(restored, header.Name)
	}
	return Result{Path: in, Files: restored, Warning: "Restore abgeschlossen. Dienste ggf. neu starten."}, nil
}

func addFile(tw *tar.Writer, path, name string) error {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = name
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err = io.Copy(tw, file)
	return err
}

func restoreTarget(cfg config.Config, name string) (string, bool) {
	if strings.Contains(name, "..") {
		return "", false
	}
	switch name {
	case "var/lib/camera-appliance/state.db":
		return cfg.DBPath(), true
	case "var/lib/camera-appliance/generated/go2rtc.yaml":
		return cfg.Go2RTCConfigPath(), true
	case "etc/camera-appliance/local.env":
		return filepath.Join(cfg.ConfigDir, "local.env"), true
	case "etc/camera-appliance/secrets.env":
		return filepath.Join(cfg.ConfigDir, "secrets.env"), true
	default:
		return "", false
	}
}

func ExplainSensitive(includeSecrets bool) string {
	if includeSecrets {
		return "Dieses Backup enthält lokale Secrets."
	}
	return "Dieses Backup enthält Kamera-Zugangsdaten aus local.env; secrets.env wird standardmäßig ausgeschlossen."
}
