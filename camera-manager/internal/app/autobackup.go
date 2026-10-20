package app

import (
	"context"
	"path/filepath"
	"time"

	"camera-appliance/camera-manager/internal/backup"
	"camera-appliance/camera-manager/internal/redaction"
)

const (
	AutoBackupEnabledKey   = "backup.auto.enabled"
	autoBackupLastErrorKey = "backup.auto.last_error"

	AutoBackupKeep     = 7
	autoBackupInterval = 24 * time.Hour
	autoBackupPoll     = time.Hour
)

type AutoBackupStatus struct {
	Enabled       bool       `json:"enabled"`
	Keep          int        `json:"keep"`
	IntervalHours int        `json:"interval_hours"`
	LastAt        *time.Time `json:"last_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}

// RunAutoBackup checks hourly and creates a backup once the newest automatic one is a day old.
func (a *App) RunAutoBackup(ctx context.Context) {
	if !waitForWatchdog(ctx, 2*time.Minute) {
		return
	}
	for {
		_, _ = a.AutoBackupOnce(ctx, time.Now().UTC())
		if !waitForWatchdog(ctx, autoBackupPoll) {
			return
		}
	}
}

// AutoBackupOnce creates and prunes automatic backups when due. Manual and pre-update backups are never pruned.
func (a *App) AutoBackupOnce(ctx context.Context, now time.Time) (bool, error) {
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		return false, err
	}
	if !boolSetting(settings, AutoBackupEnabledKey, true) {
		return false, nil
	}
	entries, err := backup.List(a.Config)
	if err != nil {
		return false, a.recordAutoBackupError(ctx, settings, err)
	}
	if last := newestAutomatic(entries); last != nil && !autoBackupDue(*last, now) {
		return false, nil
	}
	result, err := backup.Create(ctx, a.Config, backup.NewPath(a.Config, backup.KindAutomatic), false)
	if err != nil {
		return false, a.recordAutoBackupError(ctx, settings, err)
	}
	_, pruneErr := backup.Prune(a.Config, backup.KindAutomatic, AutoBackupKeep)
	_ = a.Store.AddEvent(ctx, "info", "backup.auto.created", "Automatische Sicherung erstellt", map[string]string{"file": filepath.Base(result.Path)})
	if pruneErr != nil {
		return true, a.recordAutoBackupError(ctx, settings, pruneErr)
	}
	if settings[autoBackupLastErrorKey] != "" {
		_ = a.Store.PutSettings(ctx, map[string]string{autoBackupLastErrorKey: ""})
	}
	return true, nil
}

func (a *App) AutoBackupStatus(ctx context.Context, entries []backup.Entry) (AutoBackupStatus, error) {
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		return AutoBackupStatus{}, err
	}
	status := AutoBackupStatus{
		Enabled:       boolSetting(settings, AutoBackupEnabledKey, true),
		Keep:          AutoBackupKeep,
		IntervalHours: int(autoBackupInterval / time.Hour),
		LastError:     settings[autoBackupLastErrorKey],
	}
	if last := newestAutomatic(entries); last != nil {
		status.LastAt = last
	}
	return status, nil
}

// autoBackupDue also fires when the clock went backwards past the last backup.
func autoBackupDue(last time.Time, now time.Time) bool {
	return now.Sub(last) >= autoBackupInterval || last.After(now.Add(autoBackupPoll))
}

func newestAutomatic(entries []backup.Entry) *time.Time {
	for _, entry := range entries {
		if entry.Kind == backup.KindAutomatic {
			return &entry.CreatedAt
		}
	}
	return nil
}

// recordAutoBackupError logs an event only when the error changes, so hourly retries do not flood events.
func (a *App) recordAutoBackupError(ctx context.Context, settings map[string]string, err error) error {
	message := redaction.Text(err.Error())
	if settings[autoBackupLastErrorKey] != message {
		_ = a.Store.PutSettings(ctx, map[string]string{autoBackupLastErrorKey: message})
		_ = a.Store.AddEvent(ctx, "warning", "backup.auto.failed", "Automatische Sicherung fehlgeschlagen: "+message, nil)
	}
	return err
}
