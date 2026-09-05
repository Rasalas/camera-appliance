package app

import (
	"context"
	"strings"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/relay"
	"camera-appliance/camera-manager/internal/state"
	"camera-appliance/camera-manager/internal/system"
	"camera-appliance/camera-manager/internal/version"
)

type Status struct {
	System       system.Status   `json:"system"`
	Version      version.Info    `json:"version"`
	Watchdog     WatchdogStatus  `json:"watchdog"`
	Relays       []relay.Status  `json:"relays"`
	Slots        []config.Slot   `json:"slots"`
	Bindings     []state.Binding `json:"bindings"`
	Devices      []state.Device  `json:"devices"`
	RecentEvents []state.Event   `json:"recent_events"`
	ScanRuns     []state.ScanRun `json:"scan_runs"`
}

func (a *App) Status(ctx context.Context) (Status, error) {
	bindings, err := a.Store.Bindings(ctx)
	if err != nil {
		return Status{}, err
	}
	bindings = attachSlots(bindings, a.Slots)
	devices, err := a.Store.Devices(ctx)
	if err != nil {
		return Status{}, err
	}
	events, _ := a.Store.Events(ctx, 10)
	runs, _ := a.Store.ScanRuns(ctx, 10)
	if bindings == nil {
		bindings = []state.Binding{}
	}
	if devices == nil {
		devices = []state.Device{}
	}
	if events == nil {
		events = []state.Event{}
	}
	if runs == nil {
		runs = []state.ScanRun{}
	}
	relays, _ := a.Relays().Statuses(ctx)
	return Status{
		System:       system.Check(ctx, a.Config),
		Version:      version.Current(),
		Watchdog:     a.WatchdogStatus(ctx),
		Relays:       relays,
		Slots:        a.Slots,
		Bindings:     redactBindings(bindings),
		Devices:      devices,
		RecentEvents: events,
		ScanRuns:     runs,
	}, nil
}

func redactBindings(bindings []state.Binding) []state.Binding {
	for i := range bindings {
		bindings[i].Username = strings.TrimSpace(bindings[i].Username)
	}
	return bindings
}
