import test from 'node:test'
import assert from 'node:assert/strict'
import { autoBackupSummary, backupDownloadPath, backupKindLabel, backupSizeLabel, backupTimeLabel } from './backupList.ts'

test('backup kinds use plain German labels', () => {
  assert.equal(backupKindLabel('manual'), 'Von Hand')
  assert.equal(backupKindLabel('automatic'), 'Automatisch')
  assert.equal(backupKindLabel('before-update'), 'Vor einem Update')
  assert.equal(backupKindLabel('other'), 'Unbekannt')
})

test('backup sizes are short and use German decimals', () => {
  assert.equal(backupSizeLabel(512), '512 B')
  assert.equal(backupSizeLabel(1536), '1,5 KB')
  assert.equal(backupSizeLabel(3 * 1024 * 1024), '3 MB')
  assert.equal(backupSizeLabel(250 * 1024 * 1024), '250 MB')
  assert.equal(backupSizeLabel(Number.NaN), '0 B')
})

test('backup times tolerate invalid values', () => {
  assert.equal(backupTimeLabel('nope'), 'Unbekannte Zeit')
  assert.match(backupTimeLabel('2026-01-02T12:00:00Z'), /02\.01\.2026/)
})

test('auto backup summary explains schedule and retention', () => {
  assert.equal(autoBackupSummary(undefined), '')
  assert.equal(autoBackupSummary({ enabled: false, keep: 7, interval_hours: 24 }), 'Automatische Sicherung ist aus.')
  assert.match(autoBackupSummary({ enabled: true, keep: 7, interval_hours: 24 }), /letzten 7 .*in Kürze/)
  assert.match(autoBackupSummary({ enabled: true, keep: 7, interval_hours: 24, last_at: '2026-01-02T12:00:00Z' }), /Zuletzt: 02\.01\.2026/)
})

test('download paths encode the backup name', () => {
  assert.equal(backupDownloadPath('camera-appliance-20260101-120000.tar.gz'), '/api/backups/camera-appliance-20260101-120000.tar.gz')
  assert.equal(backupDownloadPath('../state.db'), '/api/backups/..%2Fstate.db')
})
