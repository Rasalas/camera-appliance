package state

import (
	"context"
	"time"
)

const (
	historyMaxAge  = 30 * 24 * time.Hour
	historyMaxRows = 10000
)

// PruneHistory bounds the append-only history tables: events and stream checks
// older than 30 days are removed, and each history table keeps at most 10k rows.
func (s *Store) PruneHistory(ctx context.Context, now time.Time) error {
	return s.pruneHistory(ctx, now.Add(-historyMaxAge), historyMaxRows)
}

func (s *Store) pruneHistory(ctx context.Context, cutoff time.Time, maxRows int) error {
	// Scan runs are only capped, so status keeps showing the last scan however old it is.
	tables := []struct {
		name, column string
		byAge        bool
	}{
		{"events", "created_at", true},
		{"stream_checks", "checked_at", true},
		{"scan_runs", "started_at", false},
	}
	for _, table := range tables {
		if table.byAge {
			if _, err := s.db.ExecContext(ctx, `DELETE FROM `+table.name+` WHERE `+table.column+` < ?`, formatTime(cutoff)); err != nil {
				return err
			}
		}
		// Rows sharing the boundary timestamp are kept; the cap is approximate by design.
		if _, err := s.db.ExecContext(ctx, `DELETE FROM `+table.name+` WHERE `+table.column+` < (SELECT `+table.column+` FROM `+table.name+` ORDER BY `+table.column+` DESC LIMIT 1 OFFSET ?)`, maxRows-1); err != nil {
			return err
		}
	}
	return nil
}
