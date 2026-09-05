import assert from 'node:assert/strict'
import test from 'node:test'
import { generalSettingKeys, maintenanceSettingKeys, settingsPatch } from './settingsDraft.ts'

test('saving general settings preserves newer identity, layout and watchdog state', () => {
  const loaded = { 'capture_ssh_host': '', 'camera.identity.ids': 'old', 'watchdog.last_action': 'old', 'viewer.layout.mosaic': 'old', 'viewer.performance.mode': 'quality' }
  const form = { ...loaded, 'capture_ssh_host': 'relay' }
  const server = { ...loaded, 'camera.identity.ids': 'old,new', 'watchdog.last_action': 'new', 'viewer.layout.mosaic': 'new', 'viewer.performance.mode': 'low' }
  Object.assign(server, settingsPatch(form, loaded, generalSettingKeys))
  assert.equal(server['capture_ssh_host'], 'relay')
  assert.equal(server['camera.identity.ids'], 'old,new')
  assert.equal(server['watchdog.last_action'], 'new')
  assert.equal(server['viewer.layout.mosaic'], 'new')
  assert.equal(server['viewer.performance.mode'], 'low')
})

test('saving one form leaves edits to another form pending', () => {
  const baseline = { 'capture_ssh_host': '', 'watchdog.enabled': 'true' }
  const draft = { 'capture_ssh_host': 'relay', 'watchdog.enabled': 'false' }
  const saved = settingsPatch(draft, baseline, generalSettingKeys)
  Object.assign(baseline, saved)
  assert.deepEqual(saved, { 'capture_ssh_host': 'relay' })
  assert.deepEqual(settingsPatch(draft, baseline, maintenanceSettingKeys), { 'watchdog.enabled': 'false' })
})

test('number inputs produce string settings and equivalent numbers stay clean', () => {
  const baseline = { 'auth.session_hours': '12', 'watchdog.fast_interval_seconds': '30' }
  const draft = { 'auth.session_hours': 24, 'watchdog.fast_interval_seconds': 30 }
  assert.deepEqual(settingsPatch(draft, baseline, Object.keys(draft)), { 'auth.session_hours': '24' })
})
