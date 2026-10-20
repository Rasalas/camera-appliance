import assert from 'node:assert/strict'
import test from 'node:test'

import { MAX_STALL_MS, STALL_MS, createStallWatch, retryDelay, stallTimeout } from '../public/embed-watchdog.js'

const frame = (currentTime, extra = {}) => ({ currentTime, poster: '', error: false, ...extra })

test('reports connecting until the video advances, then playing', () => {
  const watch = createStallWatch(0)
  assert.deepEqual(watch.sample(0, frame(0)), { state: 'connecting', restart: false })
  assert.deepEqual(watch.sample(1000, frame(0)), { state: 'connecting', restart: false })
  assert.deepEqual(watch.sample(2000, frame(0.4)), { state: 'playing', restart: false })
})

test('detects a frozen picture after the stall window and asks for a restart', () => {
  const watch = createStallWatch(0)
  watch.sample(0, frame(0))
  watch.sample(1000, frame(1))
  assert.equal(watch.sample(1000 + STALL_MS - 1, frame(1)).state, 'playing')
  assert.deepEqual(watch.sample(1000 + STALL_MS, frame(1)), { state: 'stalled', restart: true })
})

test('treats changing MJPEG posters as progress', () => {
  const watch = createStallWatch(0)
  watch.sample(0, frame(0, { poster: 'a' }))
  assert.equal(watch.sample(STALL_MS - 1, frame(0, { poster: 'b' })).state, 'playing')
  assert.equal(watch.sample(2 * STALL_MS - 2, frame(0, { poster: 'c' })).restart, false)
})

test('backs off exponentially between restarts and caps the wait', () => {
  assert.equal(stallTimeout(0), STALL_MS)
  assert.equal(stallTimeout(1), 2 * STALL_MS)
  assert.equal(stallTimeout(5), MAX_STALL_MS)

  const watch = createStallWatch(0)
  assert.equal(watch.sample(STALL_MS, frame(0)).restart, true)
  watch.restarted(STALL_MS)
  assert.equal(watch.sample(STALL_MS + stallTimeout(1) - 1, frame(0)).restart, false)
  assert.equal(watch.sample(STALL_MS + stallTimeout(1), frame(0)).restart, true)
})

test('playback resets the backoff', () => {
  const watch = createStallWatch(0)
  watch.sample(0, frame(0))
  watch.sample(STALL_MS, frame(0))
  watch.restarted(STALL_MS)
  let now = STALL_MS + 1000
  watch.sample(now, frame(0))
  now += 1000
  assert.equal(watch.sample(now, frame(3)).state, 'playing')
  now += STALL_MS
  assert.equal(watch.sample(now, frame(3)).restart, true)
  watch.restarted(now)
  // Back to the first backoff step, not the second.
  assert.equal(watch.sample(now + stallTimeout(1), frame(0)).restart, true)
})

test('a restarted player starts from a fresh baseline', () => {
  const watch = createStallWatch(0)
  watch.sample(0, frame(0))
  watch.sample(1000, frame(5))
  watch.sample(1000 + STALL_MS, frame(5))
  watch.restarted(1000 + STALL_MS)
  assert.equal(watch.sample(2000 + STALL_MS, frame(0)).state, 'connecting')
})

test('surfaces player errors without suppressing the restart', () => {
  const watch = createStallWatch(0)
  assert.deepEqual(watch.sample(1000, frame(0, { error: true })), { state: 'error', restart: false })
  assert.deepEqual(watch.sample(STALL_MS, frame(0, { error: true })), { state: 'error', restart: true })
})

test('retries script loading with capped exponential backoff', () => {
  assert.equal(retryDelay(0), 2000)
  assert.equal(retryDelay(1), 4000)
  assert.equal(retryDelay(10), 60000)
})
