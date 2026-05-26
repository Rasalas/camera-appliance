CREATE TABLE IF NOT EXISTS slots (
  id TEXT PRIMARY KEY,
  label TEXT NOT NULL,
  role TEXT NOT NULL,
  default_stream TEXT NOT NULL,
  required INTEGER NOT NULL DEFAULT 0,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS devices (
  id TEXT PRIMARY KEY,
  first_seen_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  last_ip TEXT,
  mac_address TEXT,
  onvif_endpoint_ref TEXT,
  serial_number TEXT,
  manufacturer TEXT,
  model TEXT,
  hardware_id TEXT,
  hostname TEXT,
  raw_json TEXT
);

CREATE TABLE IF NOT EXISTS bindings (
  slot_id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL,
  label TEXT,
  username TEXT,
  stream_name TEXT NOT NULL DEFAULT 'stream2',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(slot_id) REFERENCES slots(id),
  FOREIGN KEY(device_id) REFERENCES devices(id)
);

CREATE TABLE IF NOT EXISTS scan_runs (
  id TEXT PRIMARY KEY,
  started_at TEXT NOT NULL,
  finished_at TEXT,
  status TEXT NOT NULL,
  message TEXT
);

CREATE TABLE IF NOT EXISTS stream_checks (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL,
  checked_at TEXT NOT NULL,
  stream_name TEXT NOT NULL,
  url_redacted TEXT NOT NULL,
  success INTEGER NOT NULL,
  latency_ms INTEGER,
  message TEXT,
  FOREIGN KEY(device_id) REFERENCES devices(id)
);

CREATE TABLE IF NOT EXISTS events (
  id TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  level TEXT NOT NULL,
  type TEXT NOT NULL,
  message TEXT NOT NULL,
  details_json TEXT
);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
