package state

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrNoCameraSlot = errors.New("Kein freier Kameraplatz verfügbar.")

// ConfigureCamera commits settings, name and automatic visibility together.
// A write lock precedes slot selection, including across separate Store openers.
func (s *Store) ConfigureCamera(ctx context.Context, id string, values map[string]string, name *string, visible *bool) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE devices SET id=id WHERE id=?`, id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, sql.ErrNoRows
	}
	changed := false
	if err := putSettings(ctx, tx, values); err != nil {
		return false, err
	}
	if name != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE devices SET name=nullif(?,'') WHERE id=?`, *name, id); err != nil {
			return false, err
		}
	}
	if visible != nil {
		if !*visible {
			r, err := tx.ExecContext(ctx, `DELETE FROM bindings WHERE device_id=?`, id)
			if err != nil {
				return false, err
			}
			n, _ := r.RowsAffected()
			changed = changed || n > 0
		} else {
			var slot string
			err := tx.QueryRowContext(ctx, `SELECT slot_id FROM bindings WHERE device_id=? LIMIT 1`, id).Scan(&slot)
			if errors.Is(err, sql.ErrNoRows) {
				err = tx.QueryRowContext(ctx, `SELECT id FROM slots WHERE id NOT IN (SELECT slot_id FROM bindings) ORDER BY sort_order,id LIMIT 1`).Scan(&slot)
				if errors.Is(err, sql.ErrNoRows) {
					return false, ErrNoCameraSlot
				}
				if err != nil {
					return false, err
				}
				now := formatTime(time.Now().UTC())
				_, err = tx.ExecContext(ctx, `INSERT INTO bindings(slot_id,device_id,username,stream_name,enabled,created_at,updated_at) VALUES (?,?,coalesce((SELECT value FROM settings WHERE key=?),''),coalesce((SELECT value FROM settings WHERE key=?),'stream2'),1,?,?)`, slot, id, "camera.credentials."+id+".username", "camera.credentials."+id+".stream", now, now)
				if err != nil {
					return false, err
				}
				changed = true
			} else if err != nil {
				return false, err
			} else {
				r, err := tx.ExecContext(ctx, `UPDATE bindings SET enabled=1 WHERE device_id=? AND enabled=0`, id)
				if err != nil {
					return false, err
				}
				n, _ := r.RowsAffected()
				changed = changed || n > 0
			}
		}
	}
	if changed {
		if err := putSettings(ctx, tx, map[string]string{"camera.configuration." + id + ".pending": "true"}); err != nil {
			return false, err
		}
	}
	return changed, tx.Commit()
}
