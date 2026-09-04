package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"modernc.org/sqlite"
)

func sqliteURI(path, mode string) string {
	absolute, _ := filepath.Abs(path)
	u := url.URL{Scheme: "file", Path: absolute}
	q := url.Values{"mode": {mode}, "_pragma": {"busy_timeout(5000)", "foreign_keys(1)"}}
	u.RawQuery = q.Encode()
	return u.String()
}

// Snapshot copies committed data through SQLite, including pages still in WAL.
// The caller owns the destination, which must not already exist.
func Snapshot(ctx context.Context, dbPath, destination string) error {
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := copyDatabase(ctx, dbPath, destination, false); err != nil {
		_ = os.Remove(destination)
		return err
	}
	return nil
}

// RestoreSnapshot replaces the database in a SQLite transaction. Existing
// readers and other processes keep valid connections to the same database.
func RestoreSnapshot(ctx context.Context, dbPath, source string) error {
	if err := ValidateSnapshot(ctx, source); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o750); err != nil {
		return err
	}
	if err := prepareDatabaseFile(dbPath); err != nil {
		return err
	}
	return copyDatabase(ctx, dbPath, source, true)
}

func prepareDatabaseFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	return errors.Join(file.Chmod(0o600), file.Close())
}

func ValidateSnapshot(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", sqliteURI(path, "ro"))
	if err != nil {
		return err
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("ungültiges Datenbank-Backup: %s", integrity)
	}
	// Reject unrelated or incomplete databases before touching the live state.
	for _, query := range []string{
		"SELECT id FROM devices LIMIT 0", "SELECT id FROM slots LIMIT 0",
		"SELECT slot_id,device_id FROM bindings LIMIT 0", "SELECT key,value FROM settings LIMIT 0",
	} {
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return fmt.Errorf("unvollständiges Datenbank-Backup: %w", err)
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	return nil
}

func copyDatabase(ctx context.Context, local, remote string, restore bool) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	mode := "ro"
	if restore {
		mode = "rwc"
	}
	db, err := sql.Open("sqlite", sqliteURI(local, mode))
	if err != nil {
		return err
	}
	defer db.Close()
	connection, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer connection.Close()
	return connection.Raw(func(raw any) (result error) {
		driver, ok := raw.(interface {
			NewBackup(string) (*sqlite.Backup, error)
			NewRestore(string) (*sqlite.Backup, error)
		})
		if !ok {
			return errors.New("SQLite-Treiber unterstützt keine Online-Sicherung")
		}
		var transfer *sqlite.Backup
		if restore {
			transfer, err = driver.NewRestore(sqliteURI(remote, "ro"))
		} else {
			transfer, err = driver.NewBackup(sqliteURI(remote, "rw"))
		}
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, transfer.Finish()) }()
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			more, stepErr := transfer.Step(128)
			if stepErr != nil {
				var sqliteErr *sqlite.Error
				if !errors.As(stepErr, &sqliteErr) || (sqliteErr.Code()&255 != 5 && sqliteErr.Code()&255 != 6) {
					return stepErr
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(10 * time.Millisecond):
				}
				continue
			}
			if !more {
				return nil
			}
		}
	})
}
