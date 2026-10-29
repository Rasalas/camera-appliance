package state

import (
	"context"
	"database/sql"
	"strings"
)

// PrepareRestoreSafety keeps the destination's access policy and invalidates
// archived sessions before any restored pages become visible to live readers.
// The caller holds LockAccessPolicy through preparation and publication.
func PrepareRestoreSafety(ctx context.Context, currentPath, snapshot string) error {
	current, err := Open(ctx, currentPath)
	if err != nil {
		return err
	}
	defer current.Close()
	settings, err := current.Settings(ctx)
	if err != nil {
		return err
	}
	staged, err := Open(ctx, snapshot)
	if err != nil {
		return err
	}
	defer staged.Close()
	tx, err := staged.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM auth_sessions"); err != nil {
		return err
	}
	// Authentication belongs to the destination appliance, including an explicit
	// absence of a viewer hash or local bypass. Never resurrect archived access.
	if settings["auth.admin.password_hash"] != "" {
		if _, err := tx.ExecContext(ctx, "DELETE FROM settings WHERE key LIKE 'auth.%' OR key LIKE 'network.%'"); err != nil {
			return err
		}
		for k, v := range settings {
			if strings.HasPrefix(k, "auth.") || strings.HasPrefix(k, "network.") {
				if _, err := tx.ExecContext(ctx, "INSERT INTO settings(key,value,updated_at) VALUES(?,?,datetime('now'))", k, v); err != nil {
					return err
				}
			}
		}
	}
	var hash string
	err = tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='auth.admin.password_hash'").Scan(&hash)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if hash == "" {
		if _, err := tx.ExecContext(ctx, "DELETE FROM settings WHERE key='network.lan_access_enabled'"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) lockAccessPolicy(ctx context.Context) (func(), error) {
	var path string
	if err := s.db.QueryRowContext(ctx, "SELECT file FROM pragma_database_list WHERE name='main'").Scan(&path); err != nil {
		return nil, err
	}
	return LockAccessPolicy(ctx, path)
}
