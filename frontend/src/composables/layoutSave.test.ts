import assert from 'node:assert/strict'
import test from 'node:test'

import { createSnapshotSave } from './layoutSave.ts'

function fakeTimers() {
  let next = 1
  const pending = new Map<number, () => void>()
  return {
    pending,
    schedule: (callback: () => void) => { const id = next++; pending.set(id, callback); return id },
    cancel: (id: number) => { pending.delete(id) },
    fire() { const callbacks = [...pending.values()]; pending.clear(); callbacks.forEach((callback) => callback()) }
  }
}

test('saves the layout as it was when the save was scheduled', async () => {
  const timers = fakeTimers()
  const saved: string[] = []
  const saver = createSnapshotSave({ delay: 400, save: async (value) => { saved.push(value) }, ...timers })
  const layout = { ratio: 0.3 }
  saver.schedule(layout)
  layout.ratio = 0.5 // e.g. a refresh replaced the tree before the timer fired
  timers.fire()
  await saver.idle()
  assert.deepEqual(saved, ['{"ratio":0.3}'])
})

test('flush saves immediately and only once', async () => {
  const timers = fakeTimers()
  const saved: string[] = []
  const saver = createSnapshotSave({ delay: 400, save: async (value) => { saved.push(value) }, ...timers })
  saver.schedule({ a: 1 })
  saver.schedule({ a: 2 })
  await saver.flush()
  timers.fire()
  await saver.idle()
  assert.deepEqual(saved, ['{"a":2}'])
})

test('reports busy while a save is scheduled or in flight', async () => {
  const timers = fakeTimers()
  let release: () => void = () => undefined
  const saver = createSnapshotSave({ delay: 400, save: () => new Promise<void>((resolve) => { release = resolve }), ...timers })
  assert.equal(saver.busy(), false)
  saver.schedule({ a: 1 })
  assert.equal(saver.busy(), true)
  timers.fire()
  assert.equal(saver.busy(), true)
  release()
  await saver.idle()
  assert.equal(saver.busy(), false)
})

test('a failed save does not leave the saver busy', async () => {
  const timers = fakeTimers()
  const saver = createSnapshotSave({ delay: 400, save: async () => { throw new Error('offline') }, ...timers })
  saver.schedule({ a: 1 })
  await assert.rejects(saver.flush())
  assert.equal(saver.busy(), false)
})
