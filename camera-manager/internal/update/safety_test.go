package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/backup"
)

var releaseFiles = map[string]string{
	"camera-appliance/manifest.json":                `{"version":"1.2.3","commit":"abc123"}`,
	"camera-appliance/bin/camera-appliance":         "new",
	"camera-appliance/frontend/dist/index.html":     "new dist",
	"camera-appliance/camera-manager/main.go":       "new go",
	"camera-appliance/frontend/src/pages/App.vue":   "new page",
	"camera-appliance/compose.yaml":                 "new compose",
	"camera-appliance/systemd/camera-kiosk.service": "[Service]\n",
}

func newSafetyInstall(t *testing.T) string {
	t.Helper()
	installDir := newTestInstall(t, "old")
	writeFile(t, filepath.Join(installDir, "camera-manager", "main.go"), "old go")
	writeFile(t, filepath.Join(installDir, "frontend", "src", "pages", "Old.vue"), "old page")
	return installDir
}

func stubFreeSpace(t *testing.T, free uint64) {
	t.Helper()
	original := statFree
	statFree = func(string) (uint64, uint64, error) { return free, 1, nil }
	t.Cleanup(func() { statFree = original })
}

func TestApplyRefusesWithoutFreeSpaceBeforeTouchingInstall(t *testing.T) {
	cfg := newTestConfig(t)
	installDir := newSafetyInstall(t)
	stubFreeSpace(t, 1<<20)
	restarted := false
	_, err := Apply(context.Background(), Options{
		Config: cfg, Archive: newReleaseArchive(t, releaseFiles), InstallDir: installDir,
		AutoRollback: true, Now: fixedNow,
		Restart:     func(context.Context) error { restarted = true; return nil },
		Healthcheck: func(context.Context) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "nicht genug freier Speicherplatz") || !strings.Contains(err.Error(), "nicht verändert") {
		t.Fatalf("expected German free-space refusal, got %v", err)
	}
	if restarted {
		t.Fatal("refused update must not restart services")
	}
	for path, want := range map[string]string{"bin/camera-appliance": "old", "frontend/dist/old.txt": "old", "camera-manager/main.go": "old go"} {
		if got := readFile(t, filepath.Join(installDir, path)); got != want {
			t.Errorf("%s changed to %q", path, got)
		}
	}
	if snapshots, _ := filepath.Glob(filepath.Join(cfg.BackupDir(), "rollback-*")); len(snapshots) != 0 {
		t.Fatalf("refused update must not write a snapshot: %v", snapshots)
	}
	if pathExists(filepath.Join(cfg.BackupDir(), lastUpdateFile)) {
		t.Fatal("refused update must not replace rollback metadata")
	}
}

func TestCheckFreeSpaceSumsNeedsOnSameFilesystem(t *testing.T) {
	stubFreeSpace(t, 300<<20)
	if err := checkFreeSpace(0, spaceNeed{path: "/a", bytes: 200 << 20}); err != nil {
		t.Fatal(err)
	}
	if err := checkFreeSpace(0, spaceNeed{path: "/a", bytes: 200 << 20}, spaceNeed{path: "/b", bytes: 200 << 20}); err == nil {
		t.Fatal("staging and snapshot on one disk must be counted together")
	}
}

// stubDisks maps path prefixes to {device, free bytes}; the longest prefix wins.
func stubDisks(t *testing.T, disks map[string][2]uint64) {
	t.Helper()
	original := statFree
	statFree = func(path string) (uint64, uint64, error) {
		best := ""
		for prefix := range disks {
			if strings.HasPrefix(path, prefix) && len(prefix) > len(best) {
				best = prefix
			}
		}
		if best == "" {
			return 0, 0, os.ErrNotExist
		}
		return disks[best][1], disks[best][0], nil
	}
	t.Cleanup(func() { statFree = original })
}

func TestUpdateSpaceAddsDockerBuildToSharedDisk(t *testing.T) {
	installDir, release := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(release, "bin", "camera-appliance"), "new")
	// One root filesystem: install, state and Docker data root.
	tooSmall := dockerBuildSpace + updateSpaceMargin
	stubDisks(t, map[string][2]uint64{"/": {1, tooSmall}})
	err := checkUpdateSpace(installDir, filepath.Join(installDir, "backups"), release, "/var/lib/docker")
	if err == nil || !strings.Contains(err.Error(), "Docker-Image-Build") || !strings.Contains(err.Error(), "docker builder prune") {
		t.Fatalf("expected Docker build refusal, got %v", err)
	}
	stubDisks(t, map[string][2]uint64{"/": {1, tooSmall + 1<<20}})
	if err := checkUpdateSpace(installDir, filepath.Join(installDir, "backups"), release, "/var/lib/docker"); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateSpaceChecksSeparateDockerDisk(t *testing.T) {
	installDir, release := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(release, "bin", "camera-appliance"), "new")
	stubDisks(t, map[string][2]uint64{"/docker": {2, 1 << 30}, "/": {1, 100 << 30}})
	err := checkUpdateSpace(installDir, filepath.Join(installDir, "backups"), release, "/docker")
	if err == nil || !strings.Contains(err.Error(), "auf /docker:") || !strings.Contains(err.Error(), "frei 1,0 GB") {
		t.Fatalf("expected refusal for the Docker disk, got %v", err)
	}
	if err := checkUpdateSpace(installDir, filepath.Join(installDir, "backups"), release, ""); err != nil {
		t.Fatalf("unknown Docker root must not block the update: %v", err)
	}
}

func TestApplyRefusesWhenDockerBuildDoesNotFit(t *testing.T) {
	cfg := newTestConfig(t)
	installDir := newSafetyInstall(t)
	stubDisks(t, map[string][2]uint64{"/docker": {2, 2 << 30}, "/": {1, 100 << 30}})
	stack := &fakeStack{t: t, installDir: installDir}
	ops := stack.ops()
	ops.DockerRoot = func(context.Context) (string, error) { return "/docker", nil }
	_, err := Apply(context.Background(), Options{
		Config: cfg, Archive: newReleaseArchive(t, releaseFiles), InstallDir: installDir,
		AutoRollback: true, Now: fixedNow, Stack: ops,
		Restart:     func(context.Context) error { return nil },
		Healthcheck: func(context.Context) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "Docker-Image-Build") {
		t.Fatalf("expected Docker build refusal, got %v", err)
	}
	if len(stack.calls) != 0 {
		t.Fatalf("refused update must not touch the stack: %v", stack.calls)
	}
	if got := readFile(t, filepath.Join(installDir, "camera-manager", "main.go")); got != "old go" {
		t.Fatalf("install changed: %q", got)
	}
}

func TestStagingFailureKeepsLiveTreesIntact(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read files without permission")
	}
	installDir := newSafetyInstall(t)
	release := t.TempDir()
	writeFile(t, filepath.Join(release, "bin", "camera-appliance"), "new")
	writeFile(t, filepath.Join(release, "frontend", "dist", "index.html"), "new dist")
	unreadable := filepath.Join(release, "camera-manager", "main.go")
	writeFile(t, unreadable, "new go")
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := applyRelease(context.Background(), release, installDir); err == nil {
		t.Fatal("expected copy failure")
	}
	for path, want := range map[string]string{"bin/camera-appliance": "old", "frontend/dist/old.txt": "old", "camera-manager/main.go": "old go"} {
		if got := readFile(t, filepath.Join(installDir, path)); got != want {
			t.Errorf("%s changed to %q after failed staging", path, got)
		}
	}
	if pathExists(filepath.Join(installDir, "frontend", "dist", "index.html")) {
		t.Error("partial new dist became visible")
	}
	if pathExists(filepath.Join(installDir, stagingNewDir)) || pathExists(filepath.Join(installDir, stagingOldDir)) {
		t.Error("staging leftovers remain")
	}
}

func TestSwapFailureRestoresLiveTree(t *testing.T) {
	installDir := newSafetyInstall(t)
	// Nothing staged: moving the staged tree in fails after the live tree moved aside.
	if err := swapTree(installDir, "frontend/dist"); err == nil {
		t.Fatal("expected swap failure")
	}
	if got := readFile(t, filepath.Join(installDir, "frontend", "dist", "old.txt")); got != "old" {
		t.Fatalf("live tree lost: %q", got)
	}
}

func TestRecoverStagingRepairsInterruptedSwap(t *testing.T) {
	installDir := newSafetyInstall(t)
	if err := os.MkdirAll(filepath.Join(installDir, stagingOldDir, "frontend"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(installDir, "frontend", "dist"), filepath.Join(installDir, stagingOldDir, "frontend", "dist")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(installDir, stagingNewDir, "camera-manager", "main.go"), "half staged")
	if err := recoverStaging(installDir); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(installDir, "frontend", "dist", "old.txt")); got != "old" {
		t.Fatalf("interrupted swap not repaired: %q", got)
	}
	if got := readFile(t, filepath.Join(installDir, "camera-manager", "main.go")); got != "old go" {
		t.Fatalf("staged tree leaked into live install: %q", got)
	}
	if pathExists(filepath.Join(installDir, stagingNewDir)) || pathExists(filepath.Join(installDir, stagingOldDir)) {
		t.Fatal("staging leftovers remain")
	}
}

type fakeStack struct {
	t           *testing.T
	installDir  string
	calls       []string
	restoreErr  error
	distAtBuild string
}

func (f *fakeStack) ops() StackOps {
	return StackOps{
		Build: func(context.Context) error {
			f.calls = append(f.calls, "build")
			f.distAtBuild = strings.Join(listDir(f.t, filepath.Join(f.installDir, "frontend", "dist")), ",")
			return nil
		},
		Preserve: func(context.Context) (string, error) {
			f.calls = append(f.calls, "preserve")
			return "camera-appliance-camera-manager", nil
		},
		RestorePrevious: func(_ context.Context, image string) error {
			f.calls = append(f.calls, "restore-previous:"+image)
			return f.restoreErr
		},
		PruneImages: func(context.Context) error {
			f.calls = append(f.calls, "prune")
			return nil
		},
	}
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestApplyBuildsBeforeSwappingFrontendDist(t *testing.T) {
	cfg := newTestConfig(t)
	installDir := newSafetyInstall(t)
	stack := &fakeStack{t: t, installDir: installDir}
	var calls []string
	_, err := Apply(context.Background(), Options{
		Config: cfg, Archive: newReleaseArchive(t, releaseFiles), InstallDir: installDir,
		AutoRollback: true, Now: fixedNow, Stack: stack.ops(),
		Restart: func(context.Context) error {
			calls = append(calls, "restart:"+readFile(t, filepath.Join(installDir, "frontend", "dist", "index.html")))
			return nil
		},
		Healthcheck: func(context.Context) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if stack.distAtBuild != "old.txt" {
		t.Fatalf("old backend must keep its own dist during the build, got %q", stack.distAtBuild)
	}
	if len(calls) != 1 || calls[0] != "restart:new dist" {
		t.Fatalf("restart must see the new dist: %v", calls)
	}
	if strings.Join(stack.calls, " ") != "preserve build prune" {
		t.Fatalf("stack calls = %v", stack.calls)
	}
	if got := readFile(t, filepath.Join(installDir, "camera-manager", "main.go")); got != "new go" {
		t.Fatalf("new sources not installed: %q", got)
	}
	last, err := readLastUpdate(cfg)
	if err != nil || last.PreviousImage != "camera-appliance-camera-manager" {
		t.Fatalf("preserved image not recorded: %+v %v", last, err)
	}
}

func TestApplyRollbackRetagsPreviousImageWithoutRebuild(t *testing.T) {
	cfg := newTestConfig(t)
	installDir := newSafetyInstall(t)
	stack := &fakeStack{t: t, installDir: installDir}
	restarts := 0
	checks := 0
	result, err := Apply(context.Background(), Options{
		Config: cfg, Archive: newReleaseArchive(t, releaseFiles), InstallDir: installDir,
		AutoRollback: true, Now: fixedNow, Stack: stack.ops(),
		Restart: func(context.Context) error { restarts++; return nil },
		Healthcheck: func(context.Context) error {
			checks++
			if checks == 1 {
				return errors.New("new manager offline")
			}
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "rollback was applied") {
		t.Fatalf("expected applied rollback, got %v", err)
	}
	if !result.RollbackApplied || restarts != 1 {
		t.Fatalf("rollback must not rebuild: restarts=%d result=%+v", restarts, result)
	}
	if strings.Join(stack.calls, " ") != "preserve build restore-previous:camera-appliance-camera-manager" {
		t.Fatalf("stack calls = %v", stack.calls)
	}
	for path, want := range map[string]string{"bin/camera-appliance": "old", "frontend/dist/old.txt": "old", "camera-manager/main.go": "old go"} {
		if got := readFile(t, filepath.Join(installDir, path)); got != want {
			t.Errorf("%s = %q after rollback", path, got)
		}
	}
	if pathExists(filepath.Join(installDir, "frontend", "dist", "index.html")) {
		t.Error("new dist survived rollback")
	}
}

func TestApplyRollbackRebuildsWhenPreviousImageFails(t *testing.T) {
	cfg := newTestConfig(t)
	installDir := newSafetyInstall(t)
	stack := &fakeStack{t: t, installDir: installDir, restoreErr: errors.New("previous image missing")}
	restarts := 0
	checks := 0
	result, err := Apply(context.Background(), Options{
		Config: cfg, Archive: newReleaseArchive(t, releaseFiles), InstallDir: installDir,
		AutoRollback: true, Now: fixedNow, Stack: stack.ops(),
		Restart: func(context.Context) error { restarts++; return nil },
		Healthcheck: func(context.Context) error {
			checks++
			if checks == 1 {
				return errors.New("new manager offline")
			}
			return nil
		},
	})
	if err == nil || !result.RollbackApplied || strings.Contains(err.Error(), "rollback also failed") {
		t.Fatalf("expected successful fallback rollback, got %v %+v", err, result)
	}
	if restarts != 2 {
		t.Fatalf("expected rebuild fallback, restarts=%d", restarts)
	}
}

func TestManualRollbackUsesPreservedImage(t *testing.T) {
	cfg := newTestConfig(t)
	installDir := newSafetyInstall(t)
	stack := &fakeStack{t: t, installDir: installDir}
	opts := Options{
		Config: cfg, Archive: newReleaseArchive(t, releaseFiles), InstallDir: installDir,
		AutoRollback: true, Now: fixedNow, Stack: stack.ops(),
		Restart:     func(context.Context) error { return nil },
		Healthcheck: func(context.Context) error { return nil },
	}
	if _, err := Apply(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	stack.calls = nil
	rebuilt := false
	if _, err := Rollback(context.Background(), RollbackOptions{
		Config: cfg, Stack: stack.ops(),
		Restart:     func(context.Context) error { rebuilt = true; return nil },
		Healthcheck: func(context.Context) error { return nil },
	}); err != nil {
		t.Fatal(err)
	}
	if rebuilt || strings.Join(stack.calls, " ") != "restore-previous:camera-appliance-camera-manager" {
		t.Fatalf("manual rollback rebuilt=%t calls=%v", rebuilt, stack.calls)
	}
	if got := readFile(t, filepath.Join(installDir, "camera-manager", "main.go")); got != "old go" {
		t.Fatalf("sources not restored: %q", got)
	}
}

func TestApplyKeepsNewestRollbackSnapshotsAndTheCurrentOne(t *testing.T) {
	cfg := newTestConfig(t)
	installDir := newSafetyInstall(t)
	// Snapshots dated after the current clock, e.g. after an RTC reset.
	for _, name := range []string{"rollback-20300101-000000.000000000", "rollback-20300102-000000.000000000", "rollback-20300103-000000.000000000", "rollback-20300104-000000.000000000"} {
		writeFile(t, filepath.Join(cfg.BackupDir(), name, "bin", "camera-appliance"), "older")
	}
	result, err := Apply(context.Background(), Options{
		Config: cfg, Archive: newReleaseArchive(t, releaseFiles), InstallDir: installDir,
		NoRestart: true, AutoRollback: true, Now: fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshots, _ := filepath.Glob(filepath.Join(cfg.BackupDir(), "rollback-*"))
	if len(snapshots) != backup.KeepRollbackSnapshots+1 {
		t.Fatalf("expected newest %d plus current, got %v", backup.KeepRollbackSnapshots, snapshots)
	}
	if !pathExists(filepath.Join(result.RollbackDir, "bin", "camera-appliance")) {
		t.Fatal("current rollback snapshot was pruned")
	}
	if pathExists(filepath.Join(cfg.BackupDir(), "rollback-20300101-000000.000000000")) {
		t.Fatal("oldest snapshot should be pruned")
	}
	last, err := readLastUpdate(cfg)
	if err != nil || last.RollbackDir != result.RollbackDir {
		t.Fatalf("metadata must reference the kept snapshot: %+v %v", last, err)
	}
}

func TestRepeatedUpdatesKeepThreeRollbackSnapshots(t *testing.T) {
	cfg := newTestConfig(t)
	installDir := newSafetyInstall(t)
	start := fixedNow()
	for i := 0; i < 7; i++ {
		at := start.Add(time.Duration(i) * time.Hour)
		if _, err := Apply(context.Background(), Options{
			Config: cfg, Archive: newReleaseArchive(t, releaseFiles), InstallDir: installDir,
			NoRestart: true, AutoRollback: true, Now: func() time.Time { return at },
		}); err != nil {
			t.Fatal(err)
		}
	}
	snapshots, _ := filepath.Glob(filepath.Join(cfg.BackupDir(), "rollback-*"))
	if len(snapshots) != backup.KeepRollbackSnapshots {
		t.Fatalf("expected %d snapshots, got %v", backup.KeepRollbackSnapshots, snapshots)
	}
	backups, _ := filepath.Glob(filepath.Join(cfg.BackupDir(), "camera-appliance-*.tar.gz"))
	if len(backups) != backup.KeepBackups {
		t.Fatalf("expected 5 backups, got %v", backups)
	}
}

func stubUnitHost(t *testing.T) (systemDir, home string, commands *[]string) {
	t.Helper()
	original := unitHost
	root := t.TempDir()
	systemDir = filepath.Join(root, "etc")
	home = filepath.Join(root, "home", "kiosk")
	var log []string
	unitHost.managed = func(string) bool { return true }
	unitHost.systemDir = systemDir
	unitHost.kioskGlob = filepath.Join(root, "home", "*", ".config", "systemd", "user", "camera-kiosk.service")
	unitHost.run = func(_ context.Context, name string, args ...string) error {
		log = append(log, name+" "+strings.Join(args, " "))
		return nil
	}
	t.Cleanup(func() { unitHost = original })
	return systemDir, home, &log
}

func TestRefreshInstalledUnitsOnlyReplacesChangedInstalledUnits(t *testing.T) {
	systemDir, home, commands := stubUnitHost(t)
	installDir := t.TempDir()
	writeFile(t, filepath.Join(installDir, "systemd", "camera-appliance.service"), "new unit")
	writeFile(t, filepath.Join(installDir, "systemd", "camera-kiosk.service"), "new kiosk")
	writeFile(t, filepath.Join(systemDir, "camera-appliance.service"), "old unit")
	kiosk := filepath.Join(home, ".config", "systemd", "user", "camera-kiosk.service")
	writeFile(t, kiosk, "new kiosk")

	notes := refreshInstalledUnits(context.Background(), installDir)
	if got := readFile(t, filepath.Join(systemDir, "camera-appliance.service")); got != "new unit" {
		t.Fatalf("system unit = %q", got)
	}
	if len(*commands) != 1 || (*commands)[0] != "systemctl daemon-reload" {
		t.Fatalf("commands = %v (unchanged kiosk unit must not reload)", *commands)
	}
	if len(notes) != 1 {
		t.Fatalf("notes = %v", notes)
	}

	*commands = nil
	if notes := refreshInstalledUnits(context.Background(), installDir); len(notes) != 0 || len(*commands) != 0 {
		t.Fatalf("identical units must be left alone: %v %v", notes, *commands)
	}
}

func TestRefreshInstalledUnitsNeverInstallsMissingUnits(t *testing.T) {
	systemDir, _, commands := stubUnitHost(t)
	installDir := t.TempDir()
	writeFile(t, filepath.Join(installDir, "systemd", "camera-appliance.service"), "new unit")
	if notes := refreshInstalledUnits(context.Background(), installDir); len(notes) != 0 || len(*commands) != 0 {
		t.Fatalf("unexpected refresh: %v %v", notes, *commands)
	}
	if pathExists(filepath.Join(systemDir, "camera-appliance.service")) {
		t.Fatal("unit installed on a system that never had it")
	}
}
