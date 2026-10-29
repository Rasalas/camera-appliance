package api

import (
	"testing"
	"time"
)

func TestLoginReservationsBoundWorkAndPrune(t *testing.T) {
	l := newLoginLimiter()
	now := time.Unix(100, 0)
	l.now = func() time.Time { return now }
	for i := 0; i < 4; i++ {
		if !l.Reserve("192.0.2.1:1") {
			t.Fatal("reservation denied")
		}
	}
	if l.Reserve("192.0.2.2:2") {
		t.Fatal("global work limit exceeded")
	}
	now = now.Add(2 * time.Minute)
	if l.Reserve("192.0.2.1:1") {
		t.Fatal("active work expired")
	}
	for i := 0; i < 4; i++ {
		l.RecordFailure("192.0.2.1:1", now)
		l.Release("192.0.2.1:1")
	}
	if !l.Reserve("192.0.2.2:2") {
		t.Fatal("capacity not released")
	}
	l.Release("192.0.2.2:2")
	if len(l.fails) != 1 {
		t.Fatalf("stale clients retained: %d", len(l.fails))
	}
}
