package state

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMigrateCreatesHistoryIndexes(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, index := range []string{"idx_events_created_at", "idx_stream_checks_checked_at", "idx_scan_runs_started_at"} {
		var name string
		if err := store.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='index' AND name=?`, index).Scan(&name); err != nil {
			t.Fatalf("index %s missing: %v", index, err)
		}
	}
	rows, err := store.db.QueryContext(ctx, `EXPLAIN QUERY PLAN SELECT id FROM events ORDER BY created_at DESC LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	cols, _ := rows.Columns()
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(fmt.Sprint(values...))
	}
	if !strings.Contains(plan.String(), "idx_events_created_at") {
		t.Fatalf("events query does not use the index: %q", plan.String())
	}
}

func TestPruneHistoryDropsOldRowsAndCapsRowCount(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.UpsertDevice(ctx, Device{ID: "dev1"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	insert := func(table string, at time.Time, i int) {
		t.Helper()
		id := fmt.Sprintf("%s_%d_%d", table, at.Unix(), i)
		var err error
		switch table {
		case "events":
			_, err = store.db.ExecContext(ctx, `INSERT INTO events (id,created_at,level,type,message) VALUES (?,?,'info','t','m')`, id, formatTime(at))
		case "stream_checks":
			_, err = store.db.ExecContext(ctx, `INSERT INTO stream_checks (id,device_id,checked_at,stream_name,url_redacted,success) VALUES (?,'dev1',?,'stream1','rtsp://cam/stream1',1)`, id, formatTime(at))
		case "scan_runs":
			_, err = store.db.ExecContext(ctx, `INSERT INTO scan_runs (id,started_at,status) VALUES (?,?,'finished')`, id, formatTime(at))
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	tables := []string{"events", "stream_checks", "scan_runs"}
	for _, table := range tables {
		insert(table, now.Add(-40*24*time.Hour), 0)
		for i := 0; i < 8; i++ {
			insert(table, now.Add(-time.Duration(i+1)*time.Minute), i)
		}
	}

	if err := store.pruneHistory(ctx, now.Add(-30*24*time.Hour), 5); err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		var count int
		if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 5 {
			t.Fatalf("%s: expected 5 rows after prune, got %d", table, count)
		}
	}
	events, err := store.Events(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !events[0].CreatedAt.Equal(now.Add(-time.Minute)) || !events[4].CreatedAt.Equal(now.Add(-5*time.Minute)) {
		t.Fatalf("prune kept the wrong events: newest %s oldest %s", events[0].CreatedAt, events[4].CreatedAt)
	}
}

func TestPruneHistoryKeepsLastScanRunRegardlessOfAge(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	old := formatTime(time.Now().Add(-400 * 24 * time.Hour))
	if _, err := store.db.ExecContext(ctx, `INSERT INTO scan_runs (id,started_at,status) VALUES ('old',?,'finished')`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO events (id,created_at,level,type,message) VALUES ('old',?,'info','t','m')`, old); err != nil {
		t.Fatal(err)
	}
	if err := store.PruneHistory(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	runs, err := store.ScanRuns(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatal("the last scan run should survive pruning so status can still show it")
	}
	events, err := store.Events(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatal("events older than the retention window should be pruned")
	}
}
