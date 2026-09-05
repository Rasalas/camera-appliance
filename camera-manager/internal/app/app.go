package app

import (
	"context"
	"sync"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/discovery"
	"camera-appliance/camera-manager/internal/relay"
	"camera-appliance/camera-manager/internal/secrets"
	"camera-appliance/camera-manager/internal/state"
)

type App struct {
	Config        config.Config
	Store         *state.Store
	Slots         []config.Slot
	RTSPProbe     func(ctx context.Context, host, port string) error
	Go2RTCRestart func(ctx context.Context) error
	Scan          func(context.Context, discovery.Options) ([]discovery.Result, []discovery.Subnet, error)
	discoveryMu   sync.Mutex

	// Runtime credentials are kept outside the immutable process configuration.
	camPassMu            sync.RWMutex
	cameraPassword       string
	cameraPasswordSource string
	relayOnce            sync.Once
	relays               *relay.Manager
}

// CameraCredentials returns the currently active camera password and its
// source. It is safe for concurrent use.
func (a *App) CameraCredentials() (password, source string) {
	a.camPassMu.RLock()
	defer a.camPassMu.RUnlock()
	return a.cameraPassword, a.cameraPasswordSource
}

// SetCameraCredentials updates the active camera password. It is safe for
// concurrent use.
func (a *App) SetCameraCredentials(password, source string) {
	a.camPassMu.Lock()
	defer a.camPassMu.Unlock()
	a.cameraPassword = password
	a.cameraPasswordSource = source
}

func Open(ctx context.Context) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	store, err := state.Open(ctx, cfg.DBPath())
	if err != nil {
		return nil, err
	}
	slots, err := config.LoadSlots(cfg.SlotsFile)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := store.UpsertSlots(ctx, slots); err != nil {
		_ = store.Close()
		return nil, err
	}
	a := &App{Config: cfg, Store: store, Slots: slots}
	secret := secrets.Load(cfg.ConfigDir)
	a.SetCameraCredentials(secret.Value, secret.Source)
	if err := a.applyNetworkAccess(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}
	return a, nil
}

func (a *App) Close() error {
	return a.Store.Close()
}

func (a *App) Relays() *relay.Manager {
	a.relayOnce.Do(func() { a.relays = relay.New(a.Config, a.Store, a.Slots, a.probeRTSP) })
	return a.relays
}
