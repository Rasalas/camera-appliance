package update

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// CI opts into the toolchain integration test after installing frontend deps.
// Unlike a clean checkout build, this exercises actual release replacement
// over 0.1.9 and then restores the old source snapshot before building again.
func TestReleaseTreesBuildAcrossLegacyUpgradeAndRollback(t *testing.T) {
	if os.Getenv("CAMERA_APPLIANCE_UPGRADE_BUILD_TEST") != "1" {
		t.Skip("requires npm dependencies and v0.1.9 git tag")
	}
	ctx := context.Background()
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	run := func(dir string, name string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
	}
	install := t.TempDir()
	oldArchive := filepath.Join(t.TempDir(), "old.tar")
	run(repo, "git", "archive", "-o", oldArchive, "v0.1.9", "frontend", "camera-manager")
	run(repo, "tar", "-xf", oldArchive, "-C", install)
	snapshot := t.TempDir()
	if err := snapshotInstall(ctx, install, snapshot); err != nil {
		t.Fatal(err)
	}
	release := t.TempDir()
	for _, dir := range []string{"frontend", "camera-manager"} {
		if err := copyTree(ctx, filepath.Join(repo, dir), filepath.Join(release, dir), copyOptions{ExcludeGenerated: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := copyFile(filepath.Join(repo, "LICENSE"), filepath.Join(release, "LICENSE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := applyRelease(ctx, release, install); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repo, "frontend", "node_modules"), filepath.Join(install, "frontend", "node_modules")); err != nil {
		t.Fatal(err)
	}
	run(filepath.Join(install, "frontend"), "npm", "run", "build")
	// Restore a legacy snapshot using the exact rollback path and build again.
	if err := restoreRollback(ctx, snapshot, install); err != nil {
		t.Fatal(err)
	}
	run(filepath.Join(install, "frontend"), "npm", "run", "build")
}
