package cameraaccess

import (
	"context"

	"camera-appliance/camera-manager/internal/config"
	go2rtcrender "camera-appliance/camera-manager/internal/go2rtc"
	"camera-appliance/camera-manager/internal/state"
)

// Service owns camera credential selection, probes and reference captures.
// Endpoint resolution and capture have real network/process and test adapters.
type Service struct {
	store    *state.Store
	config   config.Config
	endpoint func(context.Context, state.Device) (go2rtcrender.StreamEndpoint, error)
	Capture  func(context.Context, string, string) ([]byte, error)
}

func New(store *state.Store, cfg config.Config, endpoint func(context.Context, state.Device) (go2rtcrender.StreamEndpoint, error)) *Service {
	return &Service{store: store, config: cfg, endpoint: endpoint, Capture: captureFrame}
}

type FailureKind int

const (
	InvalidInput FailureKind = iota
	NotFound
	CaptureFailed
)

type Failure struct {
	Kind  FailureKind
	Cause error
}

func (e *Failure) Error() string { return e.Cause.Error() }

func (e *Failure) Unwrap() error { return e.Cause }

func failure(kind FailureKind, err error) error { return &Failure{Kind: kind, Cause: err} }
