package app

import (
	"context"
	"fmt"
	"strings"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/discovery"
	go2rtcrender "camera-appliance/camera-manager/internal/go2rtc"
	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/state"
	"camera-appliance/camera-manager/internal/system"
)

type App struct {
	Config config.Config
	Store  *state.Store
	Slots  []config.Slot
}

type Status struct {
	System       system.Status   `json:"system"`
	Slots        []config.Slot   `json:"slots"`
	Bindings     []state.Binding `json:"bindings"`
	Devices      []state.Device  `json:"devices"`
	RecentEvents []state.Event   `json:"recent_events"`
	ScanRuns     []state.ScanRun `json:"scan_runs"`
}

type DiscoverySummary struct {
	Run      state.ScanRun      `json:"run"`
	Subnets  []discovery.Subnet `json:"subnets"`
	Devices  []state.Device     `json:"devices"`
	Warnings []string           `json:"warnings"`
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
	return &App{Config: cfg, Store: store, Slots: slots}, nil
}

func (a *App) Close() error {
	return a.Store.Close()
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
	return Status{
		System:       system.Check(ctx, a.Config),
		Slots:        a.Slots,
		Bindings:     redactBindings(bindings),
		Devices:      devices,
		RecentEvents: events,
		ScanRuns:     runs,
	}, nil
}

func (a *App) Discover(ctx context.Context) (DiscoverySummary, error) {
	run, err := a.Store.StartScan(ctx)
	if err != nil {
		return DiscoverySummary{}, err
	}
	_ = a.Store.AddEvent(ctx, "info", "scan.started", "Kamerasuche gestartet", map[string]string{"scan_id": run.ID})
	usernames := a.usernames(ctx)
	scanner := discovery.NewScanner(discovery.Options{
		Timeout:      a.Config.RequestTimeout,
		LimitPerCIDR: a.Config.ScanLimit,
		Usernames:    usernames,
		Password:     a.Config.TapoPassword,
	})
	results, subnets, scanErr := scanner.Scan(ctx)
	var warnings []string
	if scanErr != nil {
		_ = a.Store.FinishScan(ctx, run.ID, "failed", redaction.Text(scanErr.Error()))
		_ = a.Store.AddEvent(ctx, "error", "scan.finished", "Kamerasuche fehlgeschlagen", map[string]string{"error": redaction.Text(scanErr.Error())})
		return DiscoverySummary{}, scanErr
	}
	devices := make([]state.Device, 0, len(results))
	for _, result := range results {
		device := result.Device
		if err := a.Store.UpsertDevice(ctx, device); err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		devices = append(devices, device)
		for stream, probe := range result.StreamChecks {
			_ = a.Store.SaveStreamCheck(ctx, state.StreamCheck{
				DeviceID:    device.ID,
				StreamName:  stream,
				URLRedacted: probe.URLRedacted,
				Success:     probe.Success,
				LatencyMS:   probe.LatencyMS,
				Message:     probe.Message,
			})
		}
		_ = a.Store.AddEvent(ctx, "info", "device.discovered", fmt.Sprintf("Gerät gefunden: %s", deviceLabel(device)), map[string]string{"ip": device.LastIP, "device_id": device.ID})
	}
	message := fmt.Sprintf("%d Gerät(e) gefunden", len(devices))
	_ = a.Store.FinishScan(ctx, run.ID, "finished", message)
	_ = a.Store.AddEvent(ctx, "info", "scan.finished", "Kamerasuche abgeschlossen", map[string]any{"devices": len(devices), "subnets": subnets})
	runs, _ := a.Store.ScanRuns(ctx, 1)
	if len(runs) > 0 {
		run = runs[0]
	}
	return DiscoverySummary{Run: run, Subnets: subnets, Devices: devices, Warnings: warnings}, nil
}

func (a *App) Assign(ctx context.Context, binding state.Binding) error {
	if binding.SlotID == "" || binding.DeviceID == "" {
		return fmt.Errorf("slot and device are required")
	}
	if binding.StreamName == "" {
		binding.StreamName = "stream2"
	}
	binding.Enabled = true
	if err := a.Store.UpsertBinding(ctx, binding); err != nil {
		return err
	}
	return a.Store.AddEvent(ctx, "info", "binding.updated", fmt.Sprintf("%s wurde zugeordnet", binding.SlotID), map[string]string{"slot_id": binding.SlotID, "device_id": binding.DeviceID})
}

func (a *App) RemoveBinding(ctx context.Context, slotID string) error {
	if err := a.Store.DeleteBinding(ctx, slotID); err != nil {
		return err
	}
	return a.Store.AddEvent(ctx, "info", "binding.removed", fmt.Sprintf("Zuordnung %s entfernt", slotID), nil)
}

func (a *App) RenderGo2RTC(ctx context.Context) (go2rtcrender.RenderResult, error) {
	bindings, err := a.Store.Bindings(ctx)
	if err != nil {
		return go2rtcrender.RenderResult{}, err
	}
	result, err := go2rtcrender.Render(ctx, go2rtcrender.RenderInput{
		Slots:    a.Slots,
		Bindings: bindings,
		Password: a.Config.TapoPassword,
		Output:   a.Config.Go2RTCConfigPath(),
	})
	if err != nil {
		return result, err
	}
	_ = a.Store.AddEvent(ctx, "info", "go2rtc.rendered", fmt.Sprintf("go2rtc-Konfiguration erzeugt: %d Streams", result.RenderedStreams), map[string]any{"warnings": result.Warnings})
	return result, nil
}

func (a *App) ResetBindings(ctx context.Context) error {
	if err := a.Store.ResetBindings(ctx); err != nil {
		return err
	}
	return a.Store.AddEvent(ctx, "warning", "bindings.reset", "Kamera-Zuordnungen und entdeckte Geräte wurden gelöscht", nil)
}

func (a *App) usernames(ctx context.Context) []string {
	bindings, err := a.Store.Bindings(ctx)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	for _, binding := range bindings {
		name := strings.TrimSpace(binding.Username)
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

func attachSlots(bindings []state.Binding, slots []config.Slot) []state.Binding {
	slotMap := map[string]config.Slot{}
	for _, slot := range slots {
		slotMap[slot.ID] = slot
	}
	for i := range bindings {
		if slot, ok := slotMap[bindings[i].SlotID]; ok {
			local := slot
			bindings[i].Slot = &local
		}
	}
	return bindings
}

func redactBindings(bindings []state.Binding) []state.Binding {
	for i := range bindings {
		bindings[i].Username = strings.TrimSpace(bindings[i].Username)
	}
	return bindings
}

func deviceLabel(device state.Device) string {
	if device.Manufacturer != "" || device.Model != "" {
		return strings.TrimSpace(device.Manufacturer + " " + device.Model)
	}
	if device.Hostname != "" {
		return device.Hostname
	}
	return device.LastIP
}
