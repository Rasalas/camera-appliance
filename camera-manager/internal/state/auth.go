package state

import (
	"context"
	"errors"
	"time"
)

// ReplaceAuthPassword invalidates the role's sessions in the same transaction
// as the new hash. A password change also works across CLI and server processes.
func (s *Store) ReplaceAuthPassword(ctx context.Context, key, role, hash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`, key, hash, formatTime(time.Now().UTC())); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM auth_sessions WHERE role=?", role); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveAuthSessionForPassword prevents a login verified against an old password
// from creating a new session after a concurrent password change.
func (s *Store) SaveAuthSessionForPassword(ctx context.Context, key, verifiedHash string, session AuthSession) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentHash string
	if err := tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=?", key).Scan(&currentHash); err != nil {
		return err
	}
	if currentHash != verifiedHash {
		return errors.New("Passwort wurde geändert; bitte erneut anmelden")
	}
	if err := saveAuthSession(ctx, tx, session); err != nil {
		return err
	}
	return tx.Commit()
}
