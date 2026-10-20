import assert from 'node:assert/strict'
import { test } from 'node:test'
import { restartStation } from './stationRestart.ts'

// Virtual clock: sleep advances time instantly.
function clock() {
  let now = 0
  return { now: () => now, sleep: async (ms: number) => { now += ms } }
}
const httpError = Object.assign(new Error('docker nicht verfügbar'), { kind: 'http' })
const dropped = Object.assign(new Error('weg'), { kind: 'network' })

test('waits for the station to go down and come back before reporting ready', async () => {
  const c = clock()
  const answers = [true, false, false, true]
  let probes = 0
  const outcome = await restartStation({
    ...c, trigger: () => new Promise(() => {}),
    probe: async () => answers[Math.min(probes++, answers.length - 1)]!
  })
  assert.deepEqual(outcome, { kind: 'ready' })
  assert.equal(probes, 4)
})

test('a dropped trigger connection counts as going down', async () => {
  const c = clock()
  const outcome = await restartStation({ ...c, trigger: async () => { throw dropped }, probe: async () => true })
  assert.deepEqual(outcome, { kind: 'ready' })
  assert.ok(c.now() < 10_000)
})

test('a quick restart that never looked down is accepted after the grace period', async () => {
  const c = clock()
  const outcome = await restartStation({ ...c, trigger: async () => undefined, probe: async () => true })
  assert.deepEqual(outcome, { kind: 'ready' })
  assert.ok(c.now() >= 8_000)
})

test('a rejected restart request is reported instead of waiting', async () => {
  const c = clock()
  const outcome = await restartStation({ ...c, trigger: async () => { throw httpError }, probe: async () => true })
  assert.deepEqual(outcome, { kind: 'failed', message: 'docker nicht verfügbar' })
})

test('gives up after three minutes', async () => {
  const c = clock()
  const outcome = await restartStation({ ...c, trigger: async () => { throw dropped }, probe: async () => false })
  assert.deepEqual(outcome, { kind: 'timeout' })
  assert.ok(c.now() >= 180_000)
})

test('reports progress while waiting', async () => {
  const c = clock()
  const seen: number[] = []
  await restartStation({ ...c, trigger: async () => undefined, probe: async () => true, onProgress: s => seen.push(s) })
  assert.ok(seen.length > 1)
  assert.ok(seen.every((value, i) => i === 0 || value >= seen[i - 1]!))
})
