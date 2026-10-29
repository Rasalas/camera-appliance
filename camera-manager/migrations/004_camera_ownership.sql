-- Existing dotted settings win over stale binding copies. Legacy bindings seed
-- missing keys in stable slot order, including cameras bound more than once.
INSERT OR IGNORE INTO settings(key,value,updated_at)
SELECT 'camera.credentials.' || device_id || '.username', coalesce(username,''), updated_at FROM bindings ORDER BY slot_id;
INSERT OR IGNORE INTO settings(key,value,updated_at)
SELECT 'camera.credentials.' || device_id || '.stream', stream_name, updated_at FROM bindings ORDER BY slot_id;
UPDATE bindings SET username=(SELECT value FROM settings WHERE key='camera.credentials.' || device_id || '.username'),
 stream_name=(SELECT value FROM settings WHERE key='camera.credentials.' || device_id || '.stream');
