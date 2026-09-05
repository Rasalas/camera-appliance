package snapshotupload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/redaction"
)

const schedulePrefix = "snapshot.schedule."

type QuietHours struct {
	Enabled bool   `json:"enabled"`
	Start   string `json:"start"`
	End     string `json:"end"`
}

type ScheduleInput struct {
	Enabled         bool       `json:"enabled"`
	IntervalSeconds int        `json:"interval_seconds"`
	QuietHours      QuietHours `json:"quiet_hours"`
}

type Schedule struct {
	ScheduleInput
	NextRun      *time.Time `json:"next_run,omitempty"`
	Running      bool       `json:"running"`
	LastAttempt  *time.Time `json:"last_attempt,omitempty"`
	LastSuccess  *time.Time `json:"last_success,omitempty"`
	LastFilename string     `json:"last_filename,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
}

type ScheduleStatus struct {
	Schedule
	QuietNow   bool      `json:"quiet_now"`
	DeviceTime time.Time `json:"device_time"`
	TimeZone   string    `json:"time_zone"`
}

func (q QuietHours) validate() error {
	if !q.Enabled {
		return nil
	}
	start, e1 := time.Parse("15:04", q.Start)
	end, e2 := time.Parse("15:04", q.End)
	if e1 != nil || e2 != nil || start.Equal(end) {
		return fmt.Errorf("%w: Ruhezeit benötigt verschiedene Start- und Endzeiten im Format HH:MM.", ErrInvalid)
	}
	return nil
}

func (q QuietHours) contains(at time.Time) bool {
	if !q.Enabled {
		return false
	}
	start, e1 := time.Parse("15:04", q.Start)
	end, e2 := time.Parse("15:04", q.End)
	if e1 != nil || e2 != nil {
		return false
	}
	m := at.Hour()*60 + at.Minute()
	a, b := start.Hour()*60+start.Minute(), end.Hour()*60+end.Minute()
	if a < b {
		return m >= a && m < b
	}
	return m >= a || m < b
}

// Advance by real minutes, so skipped/repeated local times on DST transitions
// obey the device clock as well. Equal start/end times are rejected at input.
func (q QuietHours) nextAllowed(at time.Time) time.Time {
	for q.contains(at) {
		at = at.Add(time.Minute).Truncate(time.Minute)
	}
	return at
}

func (q QuietHours) nextStart(at time.Time) time.Time {
	at = at.Add(time.Minute).Truncate(time.Minute)
	for !q.contains(at) {
		at = at.Add(time.Minute)
	}
	return at
}

func parseSchedule(raw string) (Schedule, error) {
	c := Schedule{ScheduleInput: ScheduleInput{IntervalSeconds: 60, QuietHours: QuietHours{Start: "22:00", End: "07:00"}}}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return c, errors.New("Gespeicherte Zeitsteuerung ist beschädigt.")
		}
	}
	if c.IntervalSeconds < 60 || c.IntervalSeconds > 86400 || c.IntervalSeconds%60 != 0 {
		return c, fmt.Errorf("%w: Intervall muss zwischen 1 und 1440 ganzen Minuten liegen.", ErrInvalid)
	}
	return c, c.QuietHours.validate()
}

func (s *Service) readSchedule(ctx context.Context, id string) (Schedule, error) {
	settings, err := s.store.Settings(ctx)
	if err != nil {
		return Schedule{}, err
	}
	return parseSchedule(settings[schedulePrefix+id])
}

func (s *Service) putSchedule(ctx context.Context, id string, c Schedule) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return s.store.PutSettings(ctx, map[string]string{schedulePrefix + id: string(data)})
}

func (s *Service) scheduleStatus(c Schedule) ScheduleStatus {
	now := s.now()
	if c.Enabled && c.NextRun != nil {
		next := *c.NextRun
		if next.Before(now) {
			next = now
		}
		next = c.QuietHours.nextAllowed(next.In(now.Location()))
		c.NextRun = &next
	}
	return ScheduleStatus{Schedule: c, QuietNow: c.Enabled && c.QuietHours.contains(now), DeviceTime: now, TimeZone: now.Format("MST (UTC-07:00)")}
}

func (s *Service) GetSchedule(ctx context.Context, id string) (ScheduleStatus, error) {
	if _, err := s.Crop(ctx, id); err != nil {
		return ScheduleStatus{}, err
	}
	s.scheduleMu.Lock()
	defer s.scheduleMu.Unlock()
	c, err := s.readSchedule(ctx, id)
	if err != nil {
		return ScheduleStatus{}, err
	}
	return s.scheduleStatus(c), nil
}

func (s *Service) SaveSchedule(ctx context.Context, id string, input ScheduleInput) (ScheduleStatus, error) {
	if _, err := s.Crop(ctx, id); err != nil {
		return ScheduleStatus{}, err
	}
	encoded, _ := json.Marshal(Schedule{ScheduleInput: input})
	if _, err := parseSchedule(string(encoded)); err != nil {
		return ScheduleStatus{}, err
	}
	if input.Enabled {
		config, err := s.Settings(ctx)
		if err != nil {
			return ScheduleStatus{}, err
		}
		if err := config.Config.Validate(); err != nil {
			return ScheduleStatus{}, fmt.Errorf("%w: %s", ErrInvalid, err)
		}
		if !config.PasswordSet {
			return ScheduleStatus{}, fmt.Errorf("%w: Bitte zuerst das Upload-Serverpasswort speichern.", ErrInvalid)
		}
	}
	s.scheduleMu.Lock()
	defer s.scheduleMu.Unlock()
	c, err := s.readSchedule(ctx, id)
	if err != nil {
		return ScheduleStatus{}, err
	}
	if c.ScheduleInput != input || (input.Enabled && c.NextRun == nil) {
		c.ScheduleInput = input
		c.NextRun = nil
		if input.Enabled {
			next := input.QuietHours.nextAllowed(s.now().Add(time.Duration(input.IntervalSeconds) * time.Second))
			c.NextRun = &next
		}
	}
	if err := s.putSchedule(ctx, id, c); err != nil {
		return ScheduleStatus{}, err
	}
	return s.scheduleStatus(c), nil
}

// Run belongs to the daemon lifecycle, not to HTTP requests or the browser.
func (s *Service) Run(ctx context.Context) {
	if !s.runner.TryLock() {
		return
	}
	defer s.runner.Unlock()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastError := ""
	recovered := false
	for {
		var err error
		if !recovered {
			err = s.recoverInterrupted(ctx)
			recovered = err == nil
		}
		if recovered {
			err = s.RunDue(ctx)
		}
		if err != nil && ctx.Err() == nil {
			message := redaction.Text(err.Error())
			if message != lastError {
				_ = s.store.AddEvent(ctx, "error", "snapshot.scheduler.failed", message, nil)
			}
			lastError = message
		} else {
			lastError = ""
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) recoverInterrupted(ctx context.Context) error {
	s.scheduleMu.Lock()
	defer s.scheduleMu.Unlock()
	settings, err := s.store.Settings(ctx)
	if err != nil {
		return err
	}
	for key, raw := range settings {
		if !strings.HasPrefix(key, schedulePrefix) {
			continue
		}
		c, err := parseSchedule(raw)
		if err != nil {
			return err
		}
		if c.Running {
			c.Running = false
			c.LastError = "Der letzte automatische Upload wurde durch einen Neustart unterbrochen."
			if err := s.putSchedule(ctx, strings.TrimPrefix(key, schedulePrefix), c); err != nil {
				return err
			}
		}
	}
	return nil
}

// RunDue is deterministic under the injected clock. Missed intervals collapse
// to one attempt, and the next due time is durable before capture begins.
func (s *Service) RunDue(ctx context.Context) error {
	if !s.runMu.TryLock() {
		return nil
	}
	defer s.runMu.Unlock()
	settings, err := s.store.Settings(ctx)
	if err != nil {
		return err
	}
	type due struct {
		id string
		at time.Time
	}
	var queue []due
	for key, raw := range settings {
		if !strings.HasPrefix(key, schedulePrefix) {
			continue
		}
		c, err := parseSchedule(raw)
		if err != nil {
			return err
		}
		if c.Enabled && c.NextRun != nil {
			queue = append(queue, due{strings.TrimPrefix(key, schedulePrefix), *c.NextRun})
		}
	}
	sort.Slice(queue, func(i, j int) bool {
		if queue[i].at.Equal(queue[j].at) {
			return queue[i].id < queue[j].id
		}
		return queue[i].at.Before(queue[j].at)
	})
	for _, item := range queue {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.runCamera(ctx, item.id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) runCamera(ctx context.Context, id string) error {
	s.scheduleMu.Lock()
	c, err := s.readSchedule(ctx, id)
	now := s.now()
	if err != nil || !c.Enabled || c.NextRun == nil || c.NextRun.After(now) || c.QuietHours.contains(now) {
		s.scheduleMu.Unlock()
		return err
	}
	if !s.busy.TryLock() {
		s.scheduleMu.Unlock()
		return nil
	}
	defer s.busy.Unlock()
	next := c.QuietHours.nextAllowed(now.Add(time.Duration(c.IntervalSeconds) * time.Second))
	c.Running, c.NextRun, c.LastAttempt = true, &next, &now
	err = s.putSchedule(ctx, id, c)
	s.scheduleMu.Unlock()
	if err != nil {
		return err
	}
	attemptCtx := ctx
	stopAtQuiet := func() {}
	if c.QuietHours.Enabled {
		attemptCtx, stopAtQuiet = context.WithTimeout(ctx, c.QuietHours.nextStart(now).Sub(now))
	}
	defer stopAtQuiet()
	crop, uploadErr := s.Crop(attemptCtx, id)
	var result Result
	if uploadErr == nil {
		result, uploadErr = s.upload(attemptCtx, id, UploadInput{Crop: &crop})
	}
	// Preserve config edits made during the transfer and record cancellation
	// before the daemon closes SQLite on shutdown.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	s.scheduleMu.Lock()
	defer s.scheduleMu.Unlock()
	c, err = s.readSchedule(finishCtx, id)
	if err != nil {
		return err
	}
	c.Running = false
	if uploadErr != nil {
		c.LastError = redaction.Text(uploadErr.Error())
		if ctx.Err() == nil && errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
			c.LastError = ""
		}
		if ctx.Err() != nil {
			c.LastError = "Der automatische Upload wurde beim Beenden unterbrochen."
		}
	} else {
		finished := s.now()
		c.LastError, c.LastFilename, c.LastSuccess = "", result.Filename, &finished
	}
	return s.putSchedule(finishCtx, id, c)
}
