package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/fingerprint"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Device struct {
	ID                string          `json:"id"`
	FirstSeenAt       time.Time       `json:"first_seen_at"`
	LastSeenAt        time.Time       `json:"last_seen_at"`
	LastIP            string          `json:"last_ip,omitempty"`
	MACAddress        string          `json:"mac_address,omitempty"`
	ONVIFEndpointRef  string          `json:"onvif_endpoint_ref,omitempty"`
	SerialNumber      string          `json:"serial_number,omitempty"`
	Manufacturer      string          `json:"manufacturer,omitempty"`
	Model             string          `json:"model,omitempty"`
	HardwareID        string          `json:"hardware_id,omitempty"`
	Hostname          string          `json:"hostname,omitempty"`
	RawJSON           json.RawMessage `json:"raw_json,omitempty"`
	StreamChecks      []StreamCheck   `json:"stream_checks,omitempty"`
	PreferredStreamOK bool            `json:"preferred_stream_ok"`
}

type Binding struct {
	SlotID     string       `json:"slot_id"`
	DeviceID   string       `json:"device_id"`
	Label      string       `json:"label,omitempty"`
	Username   string       `json:"username,omitempty"`
	StreamName string       `json:"stream_name"`
	Enabled    bool         `json:"enabled"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
	Device     *Device      `json:"device,omitempty"`
	Slot       *config.Slot `json:"slot,omitempty"`
}

type ScanRun struct {
	ID         string     `json:"id"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Status     string     `json:"status"`
	Message    string     `json:"message,omitempty"`
}

type StreamCheck struct {
	ID          string    `json:"id"`
	DeviceID    string    `json:"device_id"`
	CheckedAt   time.Time `json:"checked_at"`
	StreamName  string    `json:"stream_name"`
	URLRedacted string    `json:"url_redacted"`
	Success     bool      `json:"success"`
	LatencyMS   int64     `json:"latency_ms,omitempty"`
	Message     string    `json:"message,omitempty"`
}

type Event struct {
	ID          string          `json:"id"`
	CreatedAt   time.Time       `json:"created_at"`
	Level       string          `json:"level"`
	Type        string          `json:"type"`
	Message     string          `json:"message"`
	DetailsJSON json.RawMessage `json:"details_json,omitempty"`
}

type Setting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

func Open(ctx context.Context, dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o750); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := store.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Migrate(ctx context.Context) error {
	stmts := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA busy_timeout=5000;`,
		`CREATE TABLE IF NOT EXISTS slots (id TEXT PRIMARY KEY, label TEXT NOT NULL, role TEXT NOT NULL, default_stream TEXT NOT NULL, required INTEGER NOT NULL DEFAULT 0, sort_order INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS devices (id TEXT PRIMARY KEY, first_seen_at TEXT NOT NULL, last_seen_at TEXT NOT NULL, last_ip TEXT, mac_address TEXT, onvif_endpoint_ref TEXT, serial_number TEXT, manufacturer TEXT, model TEXT, hardware_id TEXT, hostname TEXT, raw_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS bindings (slot_id TEXT PRIMARY KEY, device_id TEXT NOT NULL, label TEXT, username TEXT, stream_name TEXT NOT NULL DEFAULT 'stream2', enabled INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, FOREIGN KEY(slot_id) REFERENCES slots(id), FOREIGN KEY(device_id) REFERENCES devices(id));`,
		`CREATE TABLE IF NOT EXISTS scan_runs (id TEXT PRIMARY KEY, started_at TEXT NOT NULL, finished_at TEXT, status TEXT NOT NULL, message TEXT);`,
		`CREATE TABLE IF NOT EXISTS stream_checks (id TEXT PRIMARY KEY, device_id TEXT NOT NULL, checked_at TEXT NOT NULL, stream_name TEXT NOT NULL, url_redacted TEXT NOT NULL, success INTEGER NOT NULL, latency_ms INTEGER, message TEXT, FOREIGN KEY(device_id) REFERENCES devices(id));`,
		`CREATE TABLE IF NOT EXISTS events (id TEXT PRIMARY KEY, created_at TEXT NOT NULL, level TEXT NOT NULL, type TEXT NOT NULL, message TEXT NOT NULL, details_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertSlots(ctx context.Context, slots []config.Slot) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, slot := range slots {
		_, err := tx.ExecContext(ctx, `INSERT INTO slots (id,label,role,default_stream,required,sort_order,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET label=excluded.label, role=excluded.role, default_stream=excluded.default_stream, required=excluded.required, sort_order=excluded.sort_order, updated_at=excluded.updated_at`,
			slot.ID, slot.Label, slot.Role, slot.DefaultStream, boolInt(slot.Required), slot.SortOrder, now, now)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Slots(ctx context.Context) ([]config.Slot, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,label,role,default_stream,required,sort_order FROM slots ORDER BY sort_order,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	slots := []config.Slot{}
	for rows.Next() {
		var slot config.Slot
		var required int
		if err := rows.Scan(&slot.ID, &slot.Label, &slot.Role, &slot.DefaultStream, &required, &slot.SortOrder); err != nil {
			return nil, err
		}
		slot.Required = required == 1
		slots = append(slots, slot)
	}
	return slots, rows.Err()
}

func (s *Store) UpsertDevice(ctx context.Context, d Device) error {
	now := time.Now().UTC()
	if d.ID == "" {
		d.ID = fingerprint.DeviceID(d.Fingerprint())
	}
	if d.FirstSeenAt.IsZero() {
		d.FirstSeenAt = now
	}
	if d.LastSeenAt.IsZero() {
		d.LastSeenAt = now
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO devices (id,first_seen_at,last_seen_at,last_ip,mac_address,onvif_endpoint_ref,serial_number,manufacturer,model,hardware_id,hostname,raw_json)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET last_seen_at=excluded.last_seen_at,last_ip=excluded.last_ip,mac_address=coalesce(nullif(excluded.mac_address,''),devices.mac_address),onvif_endpoint_ref=coalesce(nullif(excluded.onvif_endpoint_ref,''),devices.onvif_endpoint_ref),serial_number=coalesce(nullif(excluded.serial_number,''),devices.serial_number),manufacturer=coalesce(nullif(excluded.manufacturer,''),devices.manufacturer),model=coalesce(nullif(excluded.model,''),devices.model),hardware_id=coalesce(nullif(excluded.hardware_id,''),devices.hardware_id),hostname=coalesce(nullif(excluded.hostname,''),devices.hostname),raw_json=excluded.raw_json`,
		d.ID, formatTime(d.FirstSeenAt), formatTime(d.LastSeenAt), d.LastIP, d.MACAddress, d.ONVIFEndpointRef, d.SerialNumber, d.Manufacturer, d.Model, d.HardwareID, d.Hostname, string(d.RawJSON))
	return err
}

func (s *Store) Devices(ctx context.Context) ([]Device, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,first_seen_at,last_seen_at,coalesce(last_ip,''),coalesce(mac_address,''),coalesce(onvif_endpoint_ref,''),coalesce(serial_number,''),coalesce(manufacturer,''),coalesce(model,''),coalesce(hardware_id,''),coalesce(hostname,''),coalesce(raw_json,'') FROM devices ORDER BY last_seen_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	devices := []Device{}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (s *Store) Device(ctx context.Context, id string) (Device, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id,first_seen_at,last_seen_at,coalesce(last_ip,''),coalesce(mac_address,''),coalesce(onvif_endpoint_ref,''),coalesce(serial_number,''),coalesce(manufacturer,''),coalesce(model,''),coalesce(hardware_id,''),coalesce(hostname,''),coalesce(raw_json,'') FROM devices WHERE id=?`, id)
	return scanDevice(row)
}

func (s *Store) SaveStreamCheck(ctx context.Context, c StreamCheck) error {
	if c.ID == "" {
		c.ID = newID("check")
	}
	if c.CheckedAt.IsZero() {
		c.CheckedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO stream_checks (id,device_id,checked_at,stream_name,url_redacted,success,latency_ms,message) VALUES (?,?,?,?,?,?,?,?)`,
		c.ID, c.DeviceID, formatTime(c.CheckedAt), c.StreamName, c.URLRedacted, boolInt(c.Success), c.LatencyMS, c.Message)
	return err
}

func (s *Store) RecentStreamChecks(ctx context.Context, limit int) ([]StreamCheck, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,device_id,checked_at,stream_name,url_redacted,success,coalesce(latency_ms,0),coalesce(message,'') FROM stream_checks ORDER BY checked_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	checks := []StreamCheck{}
	for rows.Next() {
		var c StreamCheck
		var checked string
		var success int
		if err := rows.Scan(&c.ID, &c.DeviceID, &checked, &c.StreamName, &c.URLRedacted, &success, &c.LatencyMS, &c.Message); err != nil {
			return nil, err
		}
		c.CheckedAt = parseTime(checked)
		c.Success = success == 1
		checks = append(checks, c)
	}
	return checks, rows.Err()
}

func (s *Store) UpsertBinding(ctx context.Context, b Binding) error {
	now := time.Now().UTC()
	if b.StreamName == "" {
		b.StreamName = "stream2"
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO bindings (slot_id,device_id,label,username,stream_name,enabled,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(slot_id) DO UPDATE SET device_id=excluded.device_id,label=excluded.label,username=excluded.username,stream_name=excluded.stream_name,enabled=excluded.enabled,updated_at=excluded.updated_at`,
		b.SlotID, b.DeviceID, b.Label, b.Username, b.StreamName, boolInt(b.Enabled), formatTime(now), formatTime(now))
	return err
}

func (s *Store) Bindings(ctx context.Context) ([]Binding, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT b.slot_id,b.device_id,coalesce(b.label,''),coalesce(b.username,''),b.stream_name,b.enabled,b.created_at,b.updated_at,
		d.id,d.first_seen_at,d.last_seen_at,coalesce(d.last_ip,''),coalesce(d.mac_address,''),coalesce(d.onvif_endpoint_ref,''),coalesce(d.serial_number,''),coalesce(d.manufacturer,''),coalesce(d.model,''),coalesce(d.hardware_id,''),coalesce(d.hostname,''),coalesce(d.raw_json,'')
		FROM bindings b LEFT JOIN devices d ON d.id=b.device_id ORDER BY b.slot_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bindings := []Binding{}
	for rows.Next() {
		var b Binding
		var enabled int
		var created, updated string
		var d Device
		var first, last, raw string
		if err := rows.Scan(&b.SlotID, &b.DeviceID, &b.Label, &b.Username, &b.StreamName, &enabled, &created, &updated,
			&d.ID, &first, &last, &d.LastIP, &d.MACAddress, &d.ONVIFEndpointRef, &d.SerialNumber, &d.Manufacturer, &d.Model, &d.HardwareID, &d.Hostname, &raw); err != nil {
			return nil, err
		}
		b.Enabled = enabled == 1
		b.CreatedAt = parseTime(created)
		b.UpdatedAt = parseTime(updated)
		if d.ID != "" {
			d.FirstSeenAt = parseTime(first)
			d.LastSeenAt = parseTime(last)
			d.RawJSON = json.RawMessage(raw)
			b.Device = &d
		}
		bindings = append(bindings, b)
	}
	return bindings, rows.Err()
}

func (s *Store) DeleteBinding(ctx context.Context, slotID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM bindings WHERE slot_id=?`, slotID)
	return err
}

func (s *Store) ResetBindings(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range []string{`DELETE FROM stream_checks`, `DELETE FROM bindings`, `DELETE FROM devices`, `DELETE FROM scan_runs`} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) StartScan(ctx context.Context) (ScanRun, error) {
	run := ScanRun{ID: newID("scan"), StartedAt: time.Now().UTC(), Status: "running"}
	_, err := s.db.ExecContext(ctx, `INSERT INTO scan_runs (id,started_at,status,message) VALUES (?,?,?,?)`, run.ID, formatTime(run.StartedAt), run.Status, run.Message)
	return run, err
}

func (s *Store) FinishScan(ctx context.Context, id, status, message string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE scan_runs SET finished_at=?, status=?, message=? WHERE id=?`, formatTime(now), status, message, id)
	return err
}

func (s *Store) ScanRuns(ctx context.Context, limit int) ([]ScanRun, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,started_at,finished_at,status,coalesce(message,'') FROM scan_runs ORDER BY started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []ScanRun{}
	for rows.Next() {
		var r ScanRun
		var started string
		var finished sql.NullString
		if err := rows.Scan(&r.ID, &started, &finished, &r.Status, &r.Message); err != nil {
			return nil, err
		}
		r.StartedAt = parseTime(started)
		if finished.Valid {
			t := parseTime(finished.String)
			r.FinishedAt = &t
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func (s *Store) AddEvent(ctx context.Context, level, typ, message string, details any) error {
	var raw []byte
	if details != nil {
		raw, _ = json.Marshal(details)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO events (id,created_at,level,type,message,details_json) VALUES (?,?,?,?,?,?)`,
		newID("event"), formatTime(time.Now().UTC()), level, typ, message, string(raw))
	return err
}

func (s *Store) Events(ctx context.Context, limit int) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,created_at,level,type,message,coalesce(details_json,'') FROM events ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		var e Event
		var created, raw string
		if err := rows.Scan(&e.ID, &created, &e.Level, &e.Type, &e.Message, &raw); err != nil {
			return nil, err
		}
		e.CreatedAt = parseTime(created)
		e.DetailsJSON = json.RawMessage(raw)
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *Store) Settings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key,value FROM settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		settings[key] = value
	}
	return settings, rows.Err()
}

func (s *Store) PutSettings(ctx context.Context, values map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := formatTime(time.Now().UTC())
	for key, value := range values {
		_, err := tx.ExecContext(ctx, `INSERT INTO settings (key,value,updated_at) VALUES (?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, key, value, now)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d Device) Fingerprint() fingerprint.Fingerprint {
	return fingerprint.Normalize(fingerprint.Fingerprint{
		MACAddress:       d.MACAddress,
		ONVIFEndpointRef: d.ONVIFEndpointRef,
		SerialNumber:     d.SerialNumber,
		Manufacturer:     d.Manufacturer,
		Model:            d.Model,
		HardwareID:       d.HardwareID,
		Hostname:         d.Hostname,
		LastIP:           d.LastIP,
	})
}

type scanner interface {
	Scan(dest ...any) error
}

func scanDevice(row scanner) (Device, error) {
	var d Device
	var first, last string
	var raw sql.NullString
	err := row.Scan(&d.ID, &first, &last, &d.LastIP, &d.MACAddress, &d.ONVIFEndpointRef, &d.SerialNumber, &d.Manufacturer, &d.Model, &d.HardwareID, &d.Hostname, &raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Device{}, err
		}
		return Device{}, err
	}
	d.FirstSeenAt = parseTime(first)
	d.LastSeenAt = parseTime(last)
	if raw.Valid {
		d.RawJSON = json.RawMessage(raw.String)
	}
	return d, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, value)
	return t
}

func newID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
}
