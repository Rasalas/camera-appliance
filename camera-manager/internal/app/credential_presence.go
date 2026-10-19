package app

import (
	"sync"
	"time"

	"camera-appliance/camera-manager/internal/secrets"
)

// credentialPresenceTTL bounds staleness for writes made outside this process (CLI, keyring tools).
const credentialPresenceTTL = 60 * time.Second

// credentialPresence caches whether a camera password exists, never the password itself.
type credentialPresence struct {
	mu         sync.Mutex
	generation uint64
	entries    map[string]credentialPresenceEntry
}

type credentialPresenceEntry struct {
	present   bool
	checkedAt time.Time
}

// cameraPasswordPresent reports whether a per-camera or global camera password is available.
func (a *App) cameraPasswordPresent(deviceID string) bool {
	if password, _ := a.CameraCredentials(); password != "" {
		return true
	}
	if deviceID == "" {
		return false
	}
	c := &a.credentialPresence
	now := time.Now()
	c.mu.Lock()
	entry, ok := c.entries[deviceID]
	generation := c.generation
	c.mu.Unlock()
	if ok && now.Sub(entry.checkedAt) < credentialPresenceTTL {
		return entry.present
	}
	lookup := a.loadCameraSecret
	if lookup == nil {
		lookup = secrets.LoadCamera
	}
	present := lookup(a.Config.ConfigDir, deviceID).Value != ""
	c.mu.Lock()
	// Skip the store if credentials changed during the lookup.
	if c.generation == generation {
		if c.entries == nil {
			c.entries = map[string]credentialPresenceEntry{}
		}
		c.entries[deviceID] = credentialPresenceEntry{present: present, checkedAt: now}
	}
	c.mu.Unlock()
	return present
}

// InvalidateCredentialPresence drops cached presence for deviceID, or for all devices when empty.
func (a *App) InvalidateCredentialPresence(deviceID string) {
	c := &a.credentialPresence
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	if deviceID == "" {
		c.entries = nil
		return
	}
	delete(c.entries, deviceID)
}
