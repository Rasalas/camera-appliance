import assert from 'node:assert/strict'
import test from 'node:test'

import { offlineMessage, parseEmbedMessage, planPlayer } from './viewerPlayback.ts'
import type { PlanContext } from './viewerPlayback.ts'

const base: PlanContext = { mode: 'quality', primary: 'cam1', spotlight: '', released: new Set(), paused: false, hdStatus: {} }
const live = (alias: string, hasHD = true) => ({ alias, playable: true, hasHD })

test('renders every playable sub stream in quality mode', () => {
  assert.deepEqual(planPlayer(live('cam2'), base), { sub: true, hd: false, waiting: false })
  assert.deepEqual(planPlayer({ alias: 'cam2', playable: false, hasHD: true }, base), { sub: false, hd: false, waiting: false })
})

test('stops all players while paused', () => {
  assert.deepEqual(planPlayer(live('cam1'), { ...base, paused: true }), { sub: false, hd: false, waiting: false })
})

test('spotlight drops hidden tiles and keeps the sub stream until HD plays', () => {
  const ctx = { ...base, spotlight: 'cam2' }
  assert.deepEqual(planPlayer(live('cam1'), ctx), { sub: false, hd: false, waiting: false })
  assert.deepEqual(planPlayer(live('cam2'), ctx), { sub: true, hd: true, waiting: false })
  assert.deepEqual(planPlayer(live('cam2'), { ...ctx, hdStatus: { cam2: 'connecting' } }), { sub: true, hd: true, waiting: false })
  assert.deepEqual(planPlayer(live('cam2'), { ...ctx, hdStatus: { cam2: 'playing' } }), { sub: false, hd: true, waiting: false })
  assert.deepEqual(planPlayer(live('cam2', false), ctx), { sub: true, hd: false, waiting: false })
})

test('low mode keeps only the primary view live', () => {
  const ctx = { ...base, mode: 'low' as const }
  assert.equal(planPlayer(live('cam1'), ctx).sub, true)
  assert.equal(planPlayer(live('cam2'), ctx).sub, false)
})

test('balanced mode starts secondary views only after release', () => {
  const ctx = { ...base, mode: 'balanced' as const }
  assert.deepEqual(planPlayer(live('cam1'), ctx), { sub: true, hd: false, waiting: false })
  assert.deepEqual(planPlayer(live('cam2'), ctx), { sub: false, hd: false, waiting: true })
  assert.deepEqual(planPlayer(live('cam2'), { ...ctx, released: new Set(['cam2']) }), { sub: true, hd: false, waiting: false })
})

test('balanced mode has no stagger while a spotlight is open', () => {
  const ctx = { ...base, mode: 'balanced' as const, spotlight: 'cam2' }
  assert.deepEqual(planPlayer(live('cam2', false), ctx), { sub: true, hd: false, waiting: false })
  assert.equal(planPlayer(live('cam3'), ctx).waiting, false)
})

test('maps embed messages, including the legacy ready message', () => {
  assert.deepEqual(parseEmbedMessage({ type: 'camera-stream-state', src: 'cam1', state: 'stalled' }), { alias: 'cam1', hd: false, state: 'stalled' })
  assert.deepEqual(parseEmbedMessage({ type: 'camera-stream-ready', src: 'cam3_stream1' }), { alias: 'cam3', hd: true, state: 'playing' })
  assert.equal(parseEmbedMessage({ type: 'camera-stream-state', src: 'cam1', state: 'bogus' }), undefined)
  assert.equal(parseEmbedMessage({ type: 'camera-audio', muted: true }), undefined)
  assert.equal(parseEmbedMessage(null), undefined)
})

test('offline text is plain German and never the raw backend detail', () => {
  assert.equal(offlineMessage('offline'), 'Kamera nicht erreichbar – verbindet automatisch neu')
  assert.equal(offlineMessage('stream_unavailable'), 'Kamera nicht erreichbar – verbindet automatisch neu')
  assert.equal(offlineMessage('credentials_failed'), 'Anmeldung an der Kamera fehlgeschlagen')
})
