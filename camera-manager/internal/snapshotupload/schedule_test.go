package snapshotupload

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"camera-appliance/camera-manager/internal/state"
)

func scheduledService(t *testing.T) (*Service, *time.Time, *int) {
	t.Helper()
	s, _ := testService(t)
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	calls := 0
	s.Send = func(context.Context, Config, string, string, []byte) error { calls++; return nil }
	if _, err := s.SaveSettings(context.Background(), SettingsInput{Config: testConfig(), Password: "local-test-password"}); err != nil {
		t.Fatal(err)
	}
	return s, &now, &calls
}

func enable(t *testing.T, s *Service, interval int, quiet QuietHours) ScheduleStatus {
	t.Helper()
	c, err := s.SaveSchedule(context.Background(), "device", ScheduleInput{Enabled: true, IntervalSeconds: interval, QuietHours: quiet})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSchedulePersistsIntervalsAndDoesNotBurstAfterRestart(t *testing.T) {
	for _, interval := range []int{60, 3600} {
		t.Run((time.Duration(interval) * time.Second).String(), func(t *testing.T) {
			s, now, calls := scheduledService(t)
			c := enable(t, s, interval, QuietHours{})
			want := now.Add(time.Duration(interval) * time.Second)
			if !c.NextRun.Equal(want) {
				t.Fatalf("next run %v want %v", c.NextRun, want)
			}
			if err := s.RunDue(context.Background()); err != nil || *calls != 0 {
				t.Fatal("ran before due")
			}
			// A fresh service represents a daemon restart against the same SQLite state.
			restarted := New(s.store, s.configDir, s.capture)
			restarted.now, restarted.Send = s.now, s.Send
			got, err := restarted.GetSchedule(context.Background(), "device")
			if err != nil || !got.NextRun.Equal(want) {
				t.Fatal("restart reset schedule")
			}
			*now = want.Add(3 * time.Hour)
			if err := restarted.RunDue(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := restarted.RunDue(context.Background()); err != nil {
				t.Fatal(err)
			}
			if *calls != 1 {
				t.Fatalf("missed intervals replayed: %d", *calls)
			}
			got, _ = restarted.GetSchedule(context.Background(), "device")
			if got.Running || got.LastSuccess == nil || got.LastError != "" || !got.NextRun.Equal(now.Add(time.Duration(interval)*time.Second)) {
				t.Fatalf("bad completion %+v", got)
			}
		})
	}
}

func TestQuietHoursIncludingMidnightAndDaylightSaving(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ at, start, end, want string }{
		{"2026-09-05T21:59:00+02:00", "22:00", "07:00", "2026-09-06T07:00:00+02:00"},
		{"2026-09-05T11:59:00+02:00", "12:00", "13:00", "2026-09-05T13:00:00+02:00"},
		{"2026-03-29T01:59:00+01:00", "22:00", "02:30", "2026-03-29T03:00:00+02:00"},
		{"2026-10-25T01:59:00+02:00", "22:00", "03:00", "2026-10-25T03:00:00+01:00"},
	} {
		t.Run(tc.at, func(t *testing.T) {
			s, now, calls := scheduledService(t)
			at, _ := time.Parse(time.RFC3339, tc.at)
			*now = at.In(berlin)
			q := QuietHours{Enabled: true, Start: tc.start, End: tc.end}
			c := enable(t, s, 60, q)
			want, _ := time.Parse(time.RFC3339, tc.want)
			if !c.NextRun.Equal(want) {
				t.Fatalf("got %v want %v", c.NextRun, want)
			}
			*now = want.Add(-time.Minute).In(berlin)
			if err := s.RunDue(context.Background()); err != nil || *calls != 0 {
				t.Fatal("uploaded during quiet hours")
			}
			status, _ := s.GetSchedule(context.Background(), "device")
			if !status.QuietNow {
				t.Fatal("missing quiet status")
			}
			*now = want.In(berlin)
			if err := s.RunDue(context.Background()); err != nil || *calls != 1 {
				t.Fatal("did not resume at end boundary")
			}
		})
	}
}

func TestQuietHoursDoNotBlockManualUpload(t *testing.T) {
	s, now, calls := scheduledService(t)
	*now = now.Add(11 * time.Hour)
	c := enable(t, s, 60, QuietHours{Enabled: true, Start: "22:00", End: "07:00"})
	if !c.QuietNow {
		t.Fatal("quiet period not active")
	}
	if _, err := s.Upload(context.Background(), "device", UploadInput{Crop: &Crop{}}); err != nil || *calls != 1 {
		t.Fatalf("manual upload blocked: %v", err)
	}
	if err := s.RunDue(context.Background()); err != nil || *calls != 1 {
		t.Fatal("automatic upload ran during quiet hours")
	}
}

func TestScheduledUploadUsesLatestSavedCropAndRecordsFailure(t *testing.T) {
	s, now, _ := scheduledService(t)
	enable(t, s, 60, QuietHours{})
	crop := Crop{Enabled: true, X: 50, Width: 50, Height: 100}
	if err := s.SaveCrop(context.Background(), "device", crop); err != nil {
		t.Fatal(err)
	}
	s.Send = func(_ context.Context, _ Config, _, _ string, data []byte) error {
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width != 50 || cfg.Height != 80 {
			t.Fatalf("wrong saved crop %+v %v", cfg, err)
		}
		return errors.New("Server nicht erreichbar")
	}
	*now = now.Add(time.Minute)
	if err := s.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	c, _ := s.GetSchedule(context.Background(), "device")
	if c.LastError == "" || c.LastSuccess != nil || c.Running || c.LastAttempt == nil {
		t.Fatalf("missing failure %+v", c)
	}
	if err := s.SaveCrop(context.Background(), "device", Crop{}); err != nil {
		t.Fatal(err)
	}
	s.Send = func(_ context.Context, _ Config, _, _ string, data []byte) error {
		cfg, _ := jpeg.DecodeConfig(bytes.NewReader(data))
		if cfg.Width != 100 {
			t.Fatal("did not use newly saved full frame")
		}
		return nil
	}
	*now = now.Add(time.Minute)
	if err := s.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	c, _ = s.GetSchedule(context.Background(), "device")
	if c.LastError != "" || c.LastSuccess == nil {
		t.Fatal("recovery not recorded")
	}
}

func TestRunningScheduleExcludesManualUploadAndPreservesDisable(t *testing.T) {
	s, now, _ := scheduledService(t)
	enable(t, s, 60, QuietHours{})
	*now = now.Add(time.Minute)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	s.Send = func(context.Context, Config, string, string, []byte) error { close(entered); <-release; return nil }
	go func() { done <- s.RunDue(context.Background()) }()
	<-entered
	if _, err := s.Upload(context.Background(), "device", UploadInput{Crop: &Crop{}}); !errors.Is(err, ErrBusy) {
		t.Fatal("overlapping manual upload allowed")
	}
	if err := s.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveSchedule(context.Background(), "device", ScheduleInput{Enabled: false, IntervalSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	c, _ := s.GetSchedule(context.Background(), "device")
	if c.Enabled || c.NextRun != nil || c.Running || c.IntervalSeconds != 3600 {
		t.Fatalf("completion overwrote config edit %+v", c)
	}
}

func TestSchedulerRunsWithoutBrowserAndRecoversInterruptedClaim(t *testing.T) {
	s, now, _ := scheduledService(t)
	enable(t, s, 60, QuietHours{})
	c, _ := s.readSchedule(context.Background(), "device")
	c.Running = true
	if err := s.putSchedule(context.Background(), "device", c); err != nil {
		t.Fatal(err)
	}
	if err := s.recoverInterrupted(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, _ := s.GetSchedule(context.Background(), "device")
	if status.Running || !strings.Contains(status.LastError, "Neustart") {
		t.Fatal("interrupted claim not recovered")
	}
	*now = now.Add(time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := make(chan struct{}, 1)
	s.Send = func(context.Context, Config, string, string, []byte) error { called <- struct{}{}; return nil }
	done := make(chan struct{})
	go func() { defer close(done); s.Run(ctx) }()
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not execute due upload")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop")
	}
}

func TestSchedulerTransfersToLocalFTPAndSFTPServers(t *testing.T) {
	for _, protocol := range []string{"ftp", "sftp"} {
		t.Run(protocol, func(t *testing.T) {
			s, now, _ := scheduledService(t)
			var cfg Config
			var files <-chan map[string][]byte
			var dir string
			if protocol == "ftp" {
				cfg, files = ftpServer(t, "")
			} else {
				cfg, dir, _ = sftpServer(t)
			}
			if _, err := s.SaveSettings(context.Background(), SettingsInput{Config: cfg, Password: "local-test-password"}); err != nil {
				t.Fatal(err)
			}
			s.Send = transfer
			enable(t, s, 60, QuietHours{})
			if err := s.SaveCrop(context.Background(), "device", Crop{Enabled: true, X: 50, Y: 25, Width: 50, Height: 50}); err != nil {
				t.Fatal(err)
			}
			*now = now.Add(time.Minute)
			if err := s.RunDue(context.Background()); err != nil {
				t.Fatal(err)
			}
			status, _ := s.GetSchedule(context.Background(), "device")
			if status.LastError != "" || status.LastFilename == "" {
				t.Fatalf("upload failed %+v", status)
			}
			var data []byte
			if protocol == "ftp" {
				data = (<-files)[status.LastFilename]
			} else {
				var err error
				data, err = os.ReadFile(filepath.Join(dir, status.LastFilename))
				if err != nil {
					t.Fatal(err)
				}
			}
			image, err := jpeg.DecodeConfig(bytes.NewReader(data))
			if err != nil || image.Width != 50 || image.Height != 40 {
				t.Fatalf("bad uploaded crop %+v %v", image, err)
			}
		})
	}
}

func TestScheduleRejectsInvalidIntervalAndQuietTimes(t *testing.T) {
	s, _, _ := scheduledService(t)
	for _, c := range []ScheduleInput{{Enabled: true, IntervalSeconds: 1}, {Enabled: true, IntervalSeconds: 61}, {Enabled: true, IntervalSeconds: 60, QuietHours: QuietHours{Enabled: true, Start: "22:00", End: "22:00"}}, {Enabled: true, IntervalSeconds: 60, QuietHours: QuietHours{Enabled: true, Start: "25:00", End: "07:00"}}} {
		if _, err := s.SaveSchedule(context.Background(), "device", c); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted invalid schedule %+v: %v", c, err)
		}
	}
}

func TestCameraSchedulesAreIndependentAndBusyUploadsStayDue(t *testing.T) {
	s, now, calls := scheduledService(t)
	ctx := context.Background()
	if err := s.store.UpsertDevice(ctx, state.Device{ID: "other", LastIP: "192.0.2.2"}); err != nil {
		t.Fatal(err)
	}
	enable(t, s, 60, QuietHours{})
	if _, err := s.SaveSchedule(ctx, "other", ScheduleInput{Enabled: true, IntervalSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Minute)
	s.busy.Lock()
	if err := s.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	s.busy.Unlock()
	c, _ := s.readSchedule(ctx, "device")
	if c.LastAttempt != nil || !c.NextRun.Equal(*now) || *calls != 0 {
		t.Fatal("busy upload consumed the scheduled attempt")
	}
	if err := s.RunDue(ctx); err != nil || *calls != 1 {
		t.Fatal("minute camera did not run alone")
	}
	if _, err := s.SaveSchedule(ctx, "device", ScheduleInput{IntervalSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(59 * time.Minute)
	if err := s.RunDue(ctx); err != nil || *calls != 2 {
		t.Fatal("disabled camera ran or hourly camera did not run")
	}
	other, _ := s.GetSchedule(ctx, "other")
	if !other.Enabled || other.LastSuccess == nil {
		t.Fatal("editing first camera changed second camera")
	}
}

func TestQuietPeriodCancelsAnAutomaticTransferAtItsStart(t *testing.T) {
	s, now, _ := scheduledService(t)
	*now = time.Date(2026, 9, 5, 21, 59, 59, 800_000_000, time.UTC)
	enable(t, s, 60, QuietHours{Enabled: true, Start: "22:00", End: "07:00"})
	c, _ := s.readSchedule(context.Background(), "device")
	c.NextRun = now
	if err := s.putSchedule(context.Background(), "device", c); err != nil {
		t.Fatal(err)
	}
	started := false
	s.Send = func(ctx context.Context, _ Config, _, _ string, _ []byte) error {
		started = true
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
			t.Error("quiet hours did not stop the transfer")
			return errors.New("timeout")
		}
	}
	if err := s.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Second)
	status, _ := s.GetSchedule(context.Background(), "device")
	if !started || status.Running || status.LastSuccess != nil || !status.QuietNow || status.LastError != "" {
		t.Fatalf("quiet start handling: %+v", status)
	}
}
