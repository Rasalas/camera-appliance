package update

import (
	"errors"
	"sync"
)

var applyMu sync.Mutex

var applying bool

func beginApply() bool {
	applyMu.Lock()
	defer applyMu.Unlock()
	if applying {
		return false
	}
	applying = true
	return true
}

func endApply() {
	applyMu.Lock()
	defer applyMu.Unlock()
	applying = false
}

// EnsureSingleFlight returns an error if another update or install is already
// running. It reserves the slot until the returned release func is called.
func EnsureSingleFlight() (func(), error) {
	if !beginApply() {
		return nil, errors.New("ein anderes Update läuft bereits, bitte warten")
	}
	return endApply, nil
}
