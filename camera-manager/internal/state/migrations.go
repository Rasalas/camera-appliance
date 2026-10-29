package state

import (
	"camera-appliance/camera-manager/migrations"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"modernc.org/sqlite"
	"time"
)

func migrate(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err = enableWAL(ctx, conn); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), `ROLLBACK`)
	if _, err = conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		var applied int
		if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version=?`, name).Scan(&applied); err != nil {
			return err
		}
		if applied > 0 {
			continue
		}
		data, err := migrations.Files.ReadFile(name)
		if err != nil {
			return err
		}
		// The unversioned runtime schema already included this column.
		skip := false
		if name == "002_device_name.sql" {
			var count int
			if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('devices') WHERE name='name'`).Scan(&count); err != nil {
				return err
			}
			skip = count > 0
		}
		if !skip {
			if _, err := conn.ExecContext(ctx, string(data)); err != nil {
				return fmt.Errorf("migration %s: %w", name, err)
			}
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES (?)`, name); err != nil {
			return err
		}
	}
	_, err = conn.ExecContext(ctx, `COMMIT`)
	return err
}

// SQLite may return BUSY from journal_mode without invoking busy_timeout when
// another opener is changing the journal. Retry only lock contention, bounded
// by the caller deadline and the same five-second budget as other writes.
func enableWAL(ctx context.Context, conn *sql.Conn) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		_, err := conn.ExecContext(ctx, `PRAGMA journal_mode=WAL`)
		if err == nil {
			return nil
		}
		var sqliteErr *sqlite.Error
		if !errors.As(err, &sqliteErr) || (sqliteErr.Code()&255 != 5 && sqliteErr.Code()&255 != 6) {
			return err
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
