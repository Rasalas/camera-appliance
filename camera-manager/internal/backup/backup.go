package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/state"
)

type Result struct {
	Path    string   `json:"path"`
	Files   []string `json:"files"`
	Warning string   `json:"warning"`
}

func Create(ctx context.Context, cfg config.Config, out string, includeSecrets bool) (Result, error) {
	if out == "" {
		out = filepath.Join(cfg.BackupDir(), "camera-appliance-"+time.Now().UTC().Format("20060102-150405.000000000")+".tar.gz")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		return Result{}, err
	}
	snapshotDir, err := os.MkdirTemp(filepath.Dir(out), ".backup-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(snapshotDir)
	snapshot := filepath.Join(snapshotDir, "state.db")
	if err := state.Snapshot(ctx, cfg.DBPath(), snapshot); err != nil {
		return Result{}, err
	}
	// Backups contain camera credentials from local.env and the generated
	// go2rtc config; never create them world-readable.
	file, err := os.CreateTemp(filepath.Dir(out), ".archive-*")
	if err != nil {
		return Result{}, err
	}
	defer file.Close()
	defer os.Remove(file.Name())
	gw := gzip.NewWriter(file)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	files := []struct {
		path string
		name string
	}{
		{snapshot, "var/lib/camera-appliance/state.db"},
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
		if err := addFile(tw, item.path, item.name); err != nil {
			if errors.Is(err, os.ErrNotExist) && item.path != snapshot {
				continue
			}
			return Result{}, err
		}
		included = append(included, item.name)
	}
	warning := "Backup enthält Kamera-Zugangsdaten aus local.env und ist sensibel. Geschützt speichern."
	if includeSecrets {
		warning = "Backup enthält secrets.env mit allen Zugangsdaten und ist besonders sensibel. Geschützt speichern."
	}
	if err := tw.Close(); err != nil {
		return Result{}, err
	}
	if err := gw.Close(); err != nil {
		return Result{}, err
	}
	if err := file.Sync(); err != nil {
		return Result{}, err
	}
	if err := file.Close(); err != nil {
		return Result{}, err
	}
	if err := os.Rename(file.Name(), out); err != nil {
		return Result{}, err
	}
	return Result{Path: out, Files: included, Warning: warning}, nil
}

// Restore validates and stages the whole archive before publishing any changes.
// Database pages are restored through SQLite, never by truncating a live file.
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
	stage, err := os.MkdirTemp("", "camera-restore-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(stage)
	tr := tar.NewReader(gr)
	var restored []string
	seen := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Result{}, err
		}
		if _, ok := restoreTarget(cfg, header.Name); !ok {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return Result{}, fmt.Errorf("Backup-Eintrag ist keine reguläre Datei: %s", header.Name)
		}
		if seen[header.Name] {
			return Result{}, fmt.Errorf("doppelter Backup-Eintrag: %s", header.Name)
		}
		seen[header.Name] = true
		path := filepath.Join(stage, header.Name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return Result{}, err
		}
		out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return Result{}, err
		}
		_, copyErr := io.Copy(out, tr)
		if err := errors.Join(copyErr, out.Close()); err != nil {
			return Result{}, err
		}
		restored = append(restored, header.Name)
	}
	// Read to the gzip EOF as well, so a corrupt trailer cannot pass validation.
	if _, err := io.Copy(io.Discard, gr); err != nil {
		return Result{}, err
	}
	const databaseName = "var/lib/camera-appliance/state.db"
	if !seen[databaseName] {
		return Result{}, errors.New("Backup enthält keine Datenbank")
	}
	snapshot := filepath.Join(stage, databaseName)
	if err := state.ValidateSnapshot(ctx, snapshot); err != nil {
		return Result{}, err
	}

	// Prepare replacements on each target filesystem. Keep originals until the
	// SQLite transaction commits so an I/O error leaves existing files intact.
	type replacement struct {
		target, pending, original string
		existed                   bool
	}
	var replacements []replacement
	defer func() {
		for _, item := range replacements {
			_ = os.Remove(item.pending)
			_ = os.Remove(item.original)
		}
	}()
	for _, name := range restored {
		if name == databaseName {
			continue
		}
		target, _ := restoreTarget(cfg, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return Result{}, err
		}
		pending, err := stageReplacement(filepath.Join(stage, name), target)
		if err != nil {
			return Result{}, err
		}
		item := replacement{target: target, pending: pending}
		if _, err := os.Lstat(target); err == nil {
			item.existed = true
			item.original, err = stageReplacement(target, target)
			if err != nil {
				_ = os.Remove(pending)
				return Result{}, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			_ = os.Remove(pending)
			return Result{}, err
		}
		replacements = append(replacements, item)
	}
	rollbackFiles := func(count int) error {
		var errs []error
		for i := count - 1; i >= 0; i-- {
			item := replacements[i]
			if item.existed {
				errs = append(errs, os.Rename(item.original, item.target))
			} else {
				errs = append(errs, os.Remove(item.target))
			}
		}
		return errors.Join(errs...)
	}
	for i, item := range replacements {
		if err := os.Rename(item.pending, item.target); err != nil {
			return Result{}, errors.Join(err, rollbackFiles(i))
		}
	}
	if err := state.RestoreSnapshot(ctx, cfg.DBPath(), snapshot); err != nil {
		return Result{}, errors.Join(err, rollbackFiles(len(replacements)))
	}
	return Result{Path: in, Files: restored, Warning: "Restore abgeschlossen. Dienste neu starten, um Zugangsdaten und Streams neu zu laden."}, nil
}

func stageReplacement(source, target string) (string, error) {
	input, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("keine reguläre Datei: %s", source)
	}
	output, err := os.CreateTemp(filepath.Dir(target), ".restore-*")
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(output, input)
	syncErr := output.Sync()
	if err := errors.Join(copyErr, syncErr, output.Close()); err != nil {
		_ = os.Remove(output.Name())
		return "", err
	}
	return output.Name(), nil
}

func addFile(tw *tar.Writer, path, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("keine reguläre Datei: %s", path)
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
