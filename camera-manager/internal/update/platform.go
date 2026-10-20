package update

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func ensureCommandLink(installDir string) error {
	cleanInstallDir, err := filepath.Abs(filepath.Clean(installDir))
	if err != nil {
		return err
	}
	if os.Geteuid() != 0 || cleanInstallDir != DefaultInstallDir {
		return nil
	}
	binary := filepath.Join(cleanInstallDir, "bin", "camera-appliance")
	if !pathExists(binary) {
		return fmt.Errorf("binary not found: %s", binary)
	}
	link := "/usr/local/bin/camera-appliance"
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	if info, err := os.Lstat(link); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("%s exists and is not a symlink", link)
		}
		if err := os.Remove(link); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(binary, link)
}

func installSystemd(ctx context.Context, installDir string, noStart bool) error {
	unit := filepath.Join(installDir, "systemd", "camera-appliance.service")
	if !pathExists(unit) {
		return fmt.Errorf("systemd unit not found: %s", unit)
	}
	if err := copyFile(unit, "/etc/systemd/system/camera-appliance.service", 0o644); err != nil {
		return err
	}
	if err := runCommand(ctx, "", "systemctl", "daemon-reload"); err != nil {
		return err
	}
	args := []string{"enable", "camera-appliance.service"}
	if !noStart {
		args = []string{"enable", "--now", "camera-appliance.service"}
	}
	return runCommand(ctx, "", "systemctl", args...)
}

func installKiosk(ctx context.Context, installDir, userName string, noStart bool) error {
	account, err := lookupInstallUser(userName)
	if err != nil {
		return err
	}
	unit := filepath.Join(installDir, "systemd", "camera-kiosk.service")
	if !pathExists(unit) {
		return fmt.Errorf("kiosk unit not found: %s", unit)
	}
	userSystemdDir := filepath.Join(account.HomeDir, ".config", "systemd", "user")
	wantsDir := filepath.Join(userSystemdDir, "default.target.wants")
	if err := os.MkdirAll(wantsDir, 0o750); err != nil {
		return err
	}
	target := filepath.Join(userSystemdDir, "camera-kiosk.service")
	if err := copyFile(unit, target, 0o644); err != nil {
		return err
	}
	link := filepath.Join(wantsDir, "camera-kiosk.service")
	_ = os.Remove(link)
	if err := os.Symlink("../camera-kiosk.service", link); err != nil {
		return err
	}
	if err := chownTree(filepath.Join(account.HomeDir, ".config", "systemd"), account.UID, account.GID); err != nil {
		return err
	}
	_ = runCommand(ctx, "", "loginctl", "enable-linger", userName)
	if noStart {
		return nil
	}
	_ = runCommand(ctx, "", "runuser", "-u", userName, "--", "systemctl", "--user", "daemon-reload")
	_ = runCommand(ctx, "", "runuser", "-u", userName, "--", "systemctl", "--user", "restart", "camera-kiosk.service")
	return nil
}

// unitHost describes where installed units live; tests replace it.
var unitHost = struct {
	managed   func(installDir string) bool
	systemDir string
	kioskGlob string
	run       func(ctx context.Context, name string, args ...string) error
}{
	managed: func(installDir string) bool {
		return installDir == DefaultInstallDir && os.Geteuid() == 0 && !pathExists("/.dockerenv") && pathExists("/run/systemd/system")
	},
	systemDir: "/etc/systemd/system",
	kioskGlob: "/home/*/.config/systemd/user/camera-kiosk.service",
	run: func(ctx context.Context, name string, args ...string) error {
		return runCommand(ctx, "", name, args...)
	},
}

// refreshInstalledUnits updates units that install copied earlier when the
// release changed them. It never installs, enables or restarts a unit.
func refreshInstalledUnits(ctx context.Context, installDir string) []string {
	if !unitHost.managed(installDir) {
		return nil
	}
	var notes []string
	target := filepath.Join(unitHost.systemDir, "camera-appliance.service")
	changed, err := replaceUnit(filepath.Join(installDir, "systemd", "camera-appliance.service"), target)
	switch {
	case err != nil:
		notes = append(notes, "Systemd-Unit konnte nicht aktualisiert werden: "+err.Error())
	case changed:
		if err := unitHost.run(ctx, "systemctl", "daemon-reload"); err != nil {
			notes = append(notes, "Systemd-Unit aktualisiert, aber daemon-reload schlug fehl: "+err.Error())
		} else {
			notes = append(notes, "Systemd-Unit camera-appliance.service aktualisiert.")
		}
	}
	kioskUnits, _ := filepath.Glob(unitHost.kioskGlob)
	for _, target := range kioskUnits {
		notes = append(notes, refreshKioskUnit(ctx, filepath.Join(installDir, "systemd", "camera-kiosk.service"), target)...)
	}
	return notes
}

func refreshKioskUnit(ctx context.Context, source, target string) []string {
	info, err := os.Lstat(target)
	if err != nil {
		return nil
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	changed, err := replaceUnit(source, target)
	if err != nil {
		return []string{"Kiosk-Unit konnte nicht aktualisiert werden: " + err.Error()}
	}
	if !changed {
		return nil
	}
	uid := strconv.FormatUint(uint64(stat.Uid), 10)
	if err := os.Chown(target, int(stat.Uid), int(stat.Gid)); err != nil {
		return []string{"Kiosk-Unit aktualisiert, Besitzer konnte nicht gesetzt werden: " + err.Error()}
	}
	account, err := user.LookupId(uid)
	if err != nil {
		return []string{"Kiosk-Unit aktualisiert; Benutzer nicht ermittelbar, daemon-reload übersprungen."}
	}
	if err := unitHost.run(ctx, "runuser", "-u", account.Username, "--", "env", "XDG_RUNTIME_DIR=/run/user/"+uid, "systemctl", "--user", "daemon-reload"); err != nil {
		return []string{"Kiosk-Unit für " + account.Username + " aktualisiert, aber daemon-reload schlug fehl: " + err.Error()}
	}
	return []string{"Kiosk-Unit für " + account.Username + " aktualisiert."}
}

// replaceUnit reports false when the release has no unit, the target was never
// installed or the contents already match.
func replaceUnit(source, target string) (bool, error) {
	want, err := os.ReadFile(source)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Never write through a link someone else may control.
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%s ist keine reguläre Datei", target)
	}
	have, err := os.ReadFile(target)
	if err != nil {
		return false, err
	}
	if bytes.Equal(have, want) {
		return false, nil
	}
	return true, copyFile(source, target, 0o644)
}

func installDesktopLaunchers(installDir, userName string) error {
	account, err := lookupInstallUser(userName)
	if err != nil {
		return err
	}
	desktopDir := filepath.Join(account.HomeDir, "Desktop")
	if err := os.MkdirAll(desktopDir, 0o750); err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(installDir, "desktop"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".desktop") {
			continue
		}
		target := filepath.Join(desktopDir, entry.Name())
		if err := copyFile(filepath.Join(installDir, "desktop", entry.Name()), target, 0o755); err != nil {
			return err
		}
		if err := os.Chown(target, account.UID, account.GID); err != nil {
			return err
		}
	}
	return nil
}

type installUser struct {
	HomeDir string
	UID     int
	GID     int
}

func lookupInstallUser(userName string) (installUser, error) {
	account, err := user.Lookup(userName)
	if err != nil {
		return installUser{}, err
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		return installUser{}, err
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return installUser{}, err
	}
	if account.HomeDir == "" || !pathExists(account.HomeDir) {
		return installUser{}, fmt.Errorf("user home not found for %s", userName)
	}
	return installUser{HomeDir: account.HomeDir, UID: uid, GID: gid}, nil
}

func chownTree(root string, uid, gid int) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chown(path, uid, gid)
	})
}

func runCommand(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s failed: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
