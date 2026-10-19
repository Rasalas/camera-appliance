import assert from 'node:assert/strict'
import test from 'node:test'

import { ADMIN_IDLE_MS, KIOSK_STORAGE_KEY, createVersionWatch, resolveKioskFlag, shouldReturnToViewer, storedKioskFlag } from './viewerKiosk.ts'

function memoryStorage(initial: Record<string, string> = {}) {
  const data = new Map(Object.entries(initial))
  return {
    data,
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => { data.set(key, value) },
    removeItem: (key: string) => { data.delete(key) }
  }
}

test('kiosk=1 is remembered for later visits without the parameter', () => {
  const storage = memoryStorage()
  assert.equal(resolveKioskFlag('?kiosk=1', storage), true)
  assert.equal(storage.data.get(KIOSK_STORAGE_KEY), '1')
  assert.equal(resolveKioskFlag('', storage), true)
  assert.equal(storedKioskFlag(storage), true)
})

test('kiosk=0 clears the remembered flag', () => {
  const storage = memoryStorage({ [KIOSK_STORAGE_KEY]: '1' })
  assert.equal(resolveKioskFlag('?kiosk=0', storage), false)
  assert.equal(storage.data.has(KIOSK_STORAGE_KEY), false)
  assert.equal(resolveKioskFlag('?foo=bar', storage), false)
})

test('a broken storage still honours the URL', () => {
  const broken = {
    getItem: () => { throw new Error('denied') },
    setItem: () => { throw new Error('denied') },
    removeItem: () => { throw new Error('denied') }
  }
  assert.equal(resolveKioskFlag('?kiosk=1', broken), true)
  assert.equal(resolveKioskFlag('', broken), false)
  assert.equal(storedKioskFlag(undefined), false)
})

test('admin pages return to the viewer only when idle in kiosk mode without drafts', () => {
  const idle = { kiosk: true, onViewer: false, idleMs: ADMIN_IDLE_MS, unsaved: false }
  assert.equal(shouldReturnToViewer(idle), true)
  assert.equal(shouldReturnToViewer({ ...idle, idleMs: ADMIN_IDLE_MS - 1 }), false)
  assert.equal(shouldReturnToViewer({ ...idle, kiosk: false }), false)
  assert.equal(shouldReturnToViewer({ ...idle, onViewer: true }), false)
  assert.equal(shouldReturnToViewer({ ...idle, unsaved: true }), false)
})

test('version watch reports a change only after a baseline exists', () => {
  const watch = createVersionWatch()
  assert.equal(watch.changed(undefined), false)
  assert.equal(watch.changed('1.2.0@abc'), false)
  assert.equal(watch.changed('1.2.0@abc'), false)
  assert.equal(watch.changed(''), false)
  assert.equal(watch.changed('1.3.0@def'), true)
})
