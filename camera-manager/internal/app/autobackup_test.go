package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/backup"
)

func seedBackup(t *testing.T, a *App, prefix string, at time.Time) {
	t.Helper()
	if err := os.MkdirAll(a.Config.BackupDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	name := prefix + at.UTC().Format("20060102-150405") + ".tar.gz"
	if err := os.WriteFile(filepath.Join(a.Config.BackupDir(), name), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func countKinds(t *testing.T, a *App) map[backup.Kind]int {
	t.Helper()
	entries, err := backup.List(a.Config)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[backup.Kind]int{}
	for _, entry := range entries {
		counts[entry.Kind]++
	}
	return counts
}

func TestAutoBackupCreatesFirstBackupAndWaitsADay(t *testing.T) {
	a := newWatchdogTestApp(t)
	ctx := context.Background()
	now := time.Now().UTC()
	created, err := a.AutoBackupOnce(ctx, now)
	if err != nil || !created {
		t.Fatalf("first run created=%v err=%v", created, err)
	}
	if created, err := a.AutoBackupOnce(ctx, now.Add(23*time.Hour)); err != nil || created {
		t.Fatalf("second run within a day created=%v err=%v", created, err)
	}
	if created, err := a.AutoBackupOnce(ctx, now.Add(25*time.Hour)); err != nil || !created {
		t.Fatalf("run after a day created=%v err=%v", created, err)
	}
	if got := countKinds(t, a)[backup.KindAutomatic]; got != 2 {
		t.Fatalf("automatic backups = %d", got)
	}
	assertWatchdogEvent(t, a, "backup.auto.created")
}

func TestAutoBackupIgnoresManualBackupsWhenDeciding(t *testing.T) {
	a := newWatchdogTestApp(t)
	now := time.Now().UTC()
	seedBackup(t, a, "camera-appliance-", now.Add(-time.Hour))
	seedBackup(t, a, "camera-appliance-update-", now.Add(-time.Hour))
	if created, err := a.AutoBackupOnce(context.Background(), now); err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
}

func TestAutoBackupDisabledDoesNothing(t *testing.T) {
	a := newWatchdogTestApp(t)
	ctx := context.Background()
	if err := a.UpdateSettings(ctx, map[string]string{AutoBackupEnabledKey: "false"}); err != nil {
		t.Fatal(err)
	}
	if created, err := a.AutoBackupOnce(ctx, time.Now().UTC()); err != nil || created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if _, err := os.Stat(a.Config.BackupDir()); err == nil && len(countKinds(t, a)) != 0 {
		t.Fatal("disabled auto backup wrote a file")
	}
	status, err := a.AutoBackupStatus(ctx, nil)
	if err != nil || status.Enabled || status.Keep != AutoBackupKeep {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestAutoBackupRetentionKeepsSevenAutomaticAndAllOthers(t *testing.T) {
	a := newWatchdogTestApp(t)
	now := time.Now().UTC()
	for day := 2; day <= 10; day++ {
		at := now.Add(-time.Duration(day) * 24 * time.Hour)
		seedBackup(t, a, "camera-appliance-auto-", at)
		seedBackup(t, a, "camera-appliance-", at)
		seedBackup(t, a, "camera-appliance-update-", at)
	}
	if created, err := a.AutoBackupOnce(context.Background(), now); err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	counts := countKinds(t, a)
	if counts[backup.KindAutomatic] != AutoBackupKeep || counts[backup.KindManual] != 9 || counts[backup.KindUpdate] != 9 {
		t.Fatalf("counts = %v", counts)
	}
	entries, _ := backup.List(a.Config)
	status, err := a.AutoBackupStatus(context.Background(), entries)
	if err != nil || status.LastAt == nil || now.Sub(*status.LastAt) > time.Minute {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestAutoBackupDueAfterClockWentBackwards(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if !autoBackupDue(now.Add(48*time.Hour), now) {
		t.Fatal("future backup should not block automatic backups")
	}
	if autoBackupDue(now.Add(-time.Hour), now) {
		t.Fatal("recent backup should not be due")
	}
}

func TestAutoBackupFailureIsRecordedOnce(t *testing.T) {
	a := newWatchdogTestApp(t)
	ctx := context.Background()
	if err := os.WriteFile(a.Config.BackupDir(), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := a.AutoBackupOnce(ctx, time.Now().UTC()); err == nil {
			t.Fatal("expected failure")
		}
	}
	events, err := a.Store.Events(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	failures := 0
	for _, event := range events {
		if event.Type == "backup.auto.failed" {
			failures++
		}
	}
	status, _ := a.AutoBackupStatus(ctx, nil)
	if failures != 1 || status.LastError == "" {
		t.Fatalf("failures=%d status=%+v", failures, status)
	}
}
