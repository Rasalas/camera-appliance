package state

import (
	"context"
	"database/sql/driver"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"modernc.org/sqlite"
)

var interruptTestFunctionID atomic.Uint64

func TestStoreRemainsUsableAfterCanceledRead(t *testing.T) {
	// Signal that SQLite is stepping the query. Returning a wide row gives the
	// driver time to observe cancellation as it prepares the result.
	queryStepping := make(chan time.Time, 1)
	function := fmt.Sprintf("watchdeck_interrupt_test_%d", interruptTestFunctionID.Add(1))
	sqlite.MustRegisterScalarFunction(function, 0, func(*sqlite.FunctionContext, []driver.Value) (driver.Value, error) {
		queryStepping <- time.Now()
		return int64(1), nil
	})
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	query := "SELECT " + function + "()" + strings.Repeat(",1", 1023)
	for attempt := 0; attempt < 150; attempt++ {
		ctx, cancel := context.WithCancel(t.Context())
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			defer cancel()
			select {
			case started := <-queryStepping:
				// Vary the cancellation point across stepping and receipt.
				deadline := started.Add(time.Duration(10+attempt%20*5) * time.Microsecond)
				for time.Now().Before(deadline) {
				}
			case <-ctx.Done():
			}
		}()
		rows, err := store.db.QueryContext(ctx, query)
		cancel()
		<-finished
		if err == nil {
			_ = rows.Close()
		}
		if _, err := store.Settings(t.Context()); err != nil {
			t.Fatalf("settings after canceled query %d: %v", attempt, err)
		}
	}
}
