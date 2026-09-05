import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createCropAutosave, validUploadCrop } from './uploadCropDraft.ts'

const full = { enabled: false, x: 0, y: 0, width: 100, height: 100 }
const crop = { enabled: true, x: 20, y: 10, width: 50, height: 50 }
const pause = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

test('automatically saves only the latest valid selection after editing stops', async () => {
  const saved: typeof crop[] = []
  const autosave = createCropAutosave(async (draft) => { saved.push(draft) }, () => {}, 5)
  autosave.change(full)
  autosave.change(crop)
  await pause(20)
  assert.deepEqual(saved, [crop])
  autosave.change({ ...crop, width: 200 })
  await pause(20)
  assert.equal(saved.length, 1)
  await autosave.close()
})

test('serializes requests and retains the latest edit while an earlier save is slow', async () => {
  const saved: typeof crop[] = []
  const states: string[] = []
  let release!: () => void
  const blocked = new Promise<void>((resolve) => { release = resolve })
  const autosave = createCropAutosave(async (draft) => {
    saved.push(draft)
    if (saved.length === 1) await blocked
  }, (state) => states.push(state))
  autosave.change(full)
  const running = autosave.flush()
  autosave.change({ ...crop, x: 10 })
  autosave.change(crop)
  assert.equal(saved.length, 1)
  release()
  await running
  assert.deepEqual(saved, [full, crop])
  assert.equal(states.filter((state) => state === 'saved').length, 1)
  await autosave.close()
})

test('reports failed saves without retry loops and retries the latest edit', async () => {
  let calls = 0
  const states: string[] = []
  const saved: typeof crop[] = []
  const autosave = createCropAutosave(async (draft) => {
    calls++
    if (calls === 1) throw new Error('Offline')
    saved.push(draft)
  }, (state) => states.push(state), 5)
  autosave.change(full)
  await autosave.flush()
  await pause(20)
  assert.equal(calls, 1)
  assert.equal(states.at(-1), 'error')
  autosave.change(crop)
  await autosave.flush()
  assert.deepEqual(saved, [crop])
  assert.equal(states.at(-1), 'saved')
  await autosave.close()
})

test('flushes a pending selection on page leave without updating the removed UI', async () => {
  const saved: typeof crop[] = []
  const states: string[] = []
  const autosave = createCropAutosave(async (draft) => { saved.push(draft) }, (state) => states.push(state))
  autosave.change(crop)
  await autosave.close()
  assert.deepEqual(saved, [crop])
  assert.deepEqual(states, ['pending'])
})

test('does not label a newer invalid draft as saved when an older request completes', async () => {
  let release!: () => void
  const states: string[] = []
  const autosave = createCropAutosave(() => new Promise<void>((resolve) => { release = resolve }), (state) => states.push(state))
  autosave.change(crop)
  const running = autosave.flush()
  autosave.change({ ...crop, width: 200 })
  release()
  await running
  assert.equal(states.includes('saved'), false)
  assert.equal(validUploadCrop({ ...crop, x: NaN }), false)
  assert.equal(validUploadCrop({ ...crop, width: 0 }), false)
  assert.equal(validUploadCrop(full), true)
  await autosave.close()
})
