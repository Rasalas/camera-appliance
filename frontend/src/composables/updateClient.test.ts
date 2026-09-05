import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createUpdateClient } from './updateClient.ts'
import type { UpdateFlowStatus } from '../types/index.ts'

function fixture() {
  let remote: UpdateFlowStatus = { phase: 'ready', current_version: '0.3.0' }
  let shown: UpdateFlowStatus | undefined
  let reloads = 0, polls = 0
  const api = {
    getUpdateStatus: async () => remote,
    checkForUpdates: async () => remote,
    downloadUpdate: async () => remote,
    installUpdate: async (): Promise<unknown> => undefined
  }
  const client = createUpdateClient(api, {
    publish: value => { shown = value }, busy() {}, reload: () => { reloads++ },
    schedule: () => { polls++ }, cancel() {}
  })
  return { api, client, set: (value: UpdateFlowStatus) => { remote = value }, shown: () => shown, reloads: () => reloads, polls: () => polls }
}
const complete: UpdateFlowStatus = { phase: 'idle', current_version: '0.4.0', job: { id: 'new', phase: 'complete', result: { new_version: { version: '0.4.0' } } } }

test('reloads the old browser bundle once after the new release is verified', async () => {
  const f = fixture(); await f.client.refresh()
  f.set(complete); await f.client.install(); await f.client.refresh()
  assert.equal(f.reloads(), 1)
})

test('lost install acknowledgement keeps polling the durable job through an outage', async () => {
  const f = fixture(); await f.client.refresh()
  f.api.installUpdate = async () => { throw new TypeError('Failed to fetch') }
  f.api.getUpdateStatus = async () => { throw new TypeError('Failed to fetch') }
  await f.client.install()
  assert.equal(f.shown()?.phase, 'installing')
  assert.ok(f.polls() > 0)
  f.api.getUpdateStatus = async () => complete
  await f.client.refresh()
  assert.equal(f.reloads(), 1)
})

test('does not reload a freshly opened page or an installation still being checked', async () => {
  const fresh = fixture(); fresh.set(complete); await fresh.client.refresh(); assert.equal(fresh.reloads(), 0)
  const f = fixture(); await f.client.refresh()
  f.set({ ...complete, phase: 'installing', job: { ...complete.job!, phase: 'installing' } })
  await f.client.refresh(); assert.equal(f.reloads(), 0)
  f.set(complete); await f.client.refresh(); assert.equal(f.reloads(), 1)
})

test('reports a failed rollback job and reloads only if the browser version changed', async () => {
  const failed: UpdateFlowStatus = { phase: 'failed', current_version: '0.3.0', error: 'restart failed; restored', job: { id: 'new', phase: 'failed', result: { rollback_applied: true, old_version: { version: '0.3.0' } } } }
  const f = fixture(); await f.client.refresh(); f.set(failed); await f.client.refresh()
  assert.equal(f.shown()?.phase, 'failed'); assert.equal(f.reloads(), 0)
  const reopened = fixture(); reopened.set({ phase: 'installing', current_version: '0.4.0' }); await reopened.client.refresh()
  reopened.set(failed); await reopened.client.refresh(); assert.equal(reopened.reloads(), 1)
})

test('reconciles rejected installation with server status without resubmitting it', async () => {
  const f = fixture(); await f.client.refresh(); let submissions = 0
  f.api.installUpdate = async () => { submissions++; throw new Error('rejected') }
  await f.client.install()
  assert.equal(submissions, 1); assert.equal(f.shown()?.phase, 'ready'); assert.equal(f.polls(), 0)
})

test('ignores duplicate actions and background checks while an update is running', async () => {
  const f = fixture(); await f.client.refresh()
  let release!: () => void, checks = 0, submissions = 0
  f.api.checkForUpdates = async () => { checks++; return complete }
  f.api.installUpdate = async () => { submissions++; await new Promise<void>(resolve => { release = resolve }) }
  const installing = f.client.install(); await f.client.install(); await f.client.check()
  assert.equal(checks, 0); assert.equal(submissions, 1)
  f.set({ phase: 'installing', current_version: '0.3.0' }); release(); await installing
  await f.client.check(); assert.equal(checks, 0)
})

test('serializes status reads and ignores stale replies after a new action or unmount', async () => {
  const f = fixture(); await f.client.refresh()
  let resolve!: (value: UpdateFlowStatus) => void, reads = 0
  f.api.getUpdateStatus = () => { reads++; return new Promise(r => { resolve = r }) }
  const first = f.client.refresh(), second = f.client.refresh(); assert.equal(reads, 1)
  f.api.downloadUpdate = async () => ({ phase: 'downloading', current_version: '0.3.0' })
  await f.client.download(); resolve({ phase: 'ready', current_version: '0.3.0' }); await Promise.all([first, second])
  assert.equal(f.shown()?.phase, 'downloading')
  const late = f.client.refresh(); f.client.close(); resolve(complete); await late
  assert.equal(f.reloads(), 0); assert.equal(f.shown()?.phase, 'downloading')
})

test('the dedicated update page shares restart recovery for custom release submissions', async () => {
  const f = fixture(); await f.client.refresh(); let customSubmissions = 0
  f.api.installUpdate = async () => { throw new Error('wrong endpoint') }
  f.api.getUpdateStatus = async () => { throw new TypeError('restarting') }
  await f.client.install(async () => { customSubmissions++; throw new TypeError('lost custom acknowledgement') })
  assert.equal(customSubmissions, 1); assert.equal(f.shown()?.phase, 'installing')
  f.api.getUpdateStatus = async () => complete; await f.client.refresh(); assert.equal(f.reloads(), 1)
})
