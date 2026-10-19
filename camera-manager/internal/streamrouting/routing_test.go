package streamrouting_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	routing "camera-appliance/camera-manager/internal/streamrouting"
)

func TestAssessSeparatesObservationsFromPersistedFailoverState(t *testing.T) {
	paths := []routing.StreamPath{{ID: "direct", Kind: routing.PathKindDirect, Host: "camera", Port: "554"}, {ID: "relay:nas", Kind: routing.PathKindRelay, Host: "host.docker.internal", Port: "15541"}}
	original := append([]routing.StreamPath(nil), paths...)
	settings := map[string]string{"camera.active_path.device.id": "direct"}
	probe := func(_ context.Context, host, port string) error {
		if host == "camera" {
			return errors.New("refused")
		}
		if host != "127.0.0.1" {
			t.Fatalf("invalid local probe host %s", host)
		}
		return nil
	}
	input := routing.Input{DeviceID: "device", Paths: paths, Settings: settings, CheckedAt: time.Date(2026, 9, 5, 1, 0, 0, 0, time.UTC)}
	observed, values := routing.Assess(context.Background(), input, probe)
	if len(values) != 0 || observed.Selected == nil || observed.Selected.ID != "direct" {
		t.Fatalf("read-only observation changed selection: %+v %v", observed, values)
	}
	input.UpdateState = true
	first, values := routing.Assess(context.Background(), input, probe)
	if first.Selected.ID != "direct" || first.Paths[0].FailureCount != 1 {
		t.Fatalf("first failure switched early: %+v", first)
	}
	for k, v := range values {
		settings[k] = v
	}
	second, values := routing.Assess(context.Background(), input, probe)
	if second.Selected.ID != "relay:nas" || second.SwitchReason != "active_failed_2" {
		t.Fatalf("failover did not honor threshold: %+v", second)
	}
	if settings["camera.active_path.device.id"] != "direct" {
		t.Fatal("evaluator persisted an active path without the caller")
	}
	if !reflect.DeepEqual(paths, original) {
		t.Fatal("evaluator mutated shared path candidates")
	}
}

func TestAssessHonorsPolicyBeforeProbing(t *testing.T) {
	input := routing.Input{DeviceID: "device", Paths: []routing.StreamPath{{ID: "direct", Kind: routing.PathKindDirect, Host: "camera", Port: "554"}, {ID: "relay:nas", Kind: routing.PathKindRelay, Host: "relay", Port: "1234"}}, Settings: map[string]string{"camera.path_policy.device": "relay_only"}, CheckedAt: time.Now()}
	calls := 0
	got, _ := routing.Assess(context.Background(), input, func(_ context.Context, host, port string) error {
		calls++
		if host != "relay" {
			t.Fatal("policy-excluded direct path was probed")
		}
		return nil
	})
	if calls != 1 || got.Selected == nil || got.Selected.ID != "relay:nas" || len(got.Paths) != 1 {
		t.Fatalf("policy %+v calls=%d", got, calls)
	}
}

func TestLastObservationReadsPersistedWatchdogCounters(t *testing.T) {
	checkedAt := time.Date(2026, 9, 5, 1, 0, 0, 0, time.UTC)
	paths := []routing.StreamPath{{ID: "direct", Kind: routing.PathKindDirect, Host: "camera", Port: "554"}, {ID: "relay:nas", Kind: routing.PathKindRelay, Host: "relay", Port: "1234"}}
	settings := map[string]string{}
	if _, ok := routing.LastObservation(settings, "device", "direct"); ok {
		t.Fatal("expected no observation before the first watchdog check")
	}
	_, values := routing.Assess(context.Background(), routing.Input{DeviceID: "device", Paths: paths, Settings: settings, UpdateState: true, CheckedAt: checkedAt}, func(_ context.Context, host, _ string) error {
		if host == "camera" {
			return errors.New("refused")
		}
		return nil
	})
	for k, v := range values {
		settings[k] = v
	}
	direct, ok := routing.LastObservation(settings, "device", "direct")
	if !ok || direct.OK || !direct.At.Equal(checkedAt) {
		t.Fatalf("direct observation = %+v, %v", direct, ok)
	}
	relay, ok := routing.LastObservation(settings, "device", "relay:nas")
	if !ok || !relay.OK || !relay.At.Equal(checkedAt) {
		t.Fatalf("relay observation = %+v, %v", relay, ok)
	}
}
