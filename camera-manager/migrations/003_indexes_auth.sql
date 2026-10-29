CREATE TABLE IF NOT EXISTS auth_sessions (token_hash TEXT PRIMARY KEY, role TEXT NOT NULL, created_at TEXT NOT NULL, expires_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_expires_at ON auth_sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at);
CREATE INDEX IF NOT EXISTS idx_stream_checks_checked_at ON stream_checks(checked_at);
CREATE INDEX IF NOT EXISTS idx_scan_runs_started_at ON scan_runs(started_at);
