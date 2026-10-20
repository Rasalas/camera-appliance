import assert from 'node:assert/strict'
import { afterEach, beforeEach, mock, test } from 'node:test'
import { ApiError, api, invalidateAuthStatus, request, setUnauthorizedHandler } from './client.ts'

type Handler = (input: string, init: RequestInit) => Promise<Response>
const realFetch = globalThis.fetch
let calls: Array<{ path: string; init: RequestInit }> = []

function stub(handler: Handler) {
  calls = []
  globalThis.fetch = (async (input: string, init: RequestInit) => {
    calls.push({ path: input, init })
    return handler(input, init)
  }) as typeof fetch
}
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
// Never answers; rejects once the request signal aborts, like a real fetch.
const hang: Handler = (_input, init) => new Promise((_resolve, reject) => {
  init.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')))
})
const auth = { enabled: true, authenticated: false, admin_password_set: true, viewer_password_set: false, viewer_public: true, local_admin_bypass: false, session_hours: 12 }

beforeEach(() => { invalidateAuthStatus(); setUnauthorizedHandler(undefined) })
afterEach(() => { globalThis.fetch = realFetch; mock.timers.reset() })

test('a hung request times out with a German error instead of blocking forever', async () => {
  stub(hang)
  const err = await request('/api/viewer', undefined, { timeoutMs: 20 }).catch(e => e)
  assert.ok(err instanceof ApiError)
  assert.ok(err instanceof Error)
  assert.equal(err.kind, 'timeout')
  assert.match(err.message, /Station antwortet nicht/)
})

test('slow endpoints get a longer timeout than the default', async () => {
  let signal: AbortSignal | undefined
  stub(async (_path, init) => { signal = init.signal ?? undefined; return json({ devices: [], subnets: [] }) })
  mock.timers.enable({ apis: ['setTimeout'] })
  const pending = api.discover()
  mock.timers.tick(30_000)
  await pending
  assert.equal(signal?.aborted, false)
})

test('a caller-supplied signal still cancels the request', async () => {
  stub(hang)
  const controller = new AbortController()
  const pending = request('/api/status', { signal: controller.signal }).catch(e => e)
  controller.abort()
  const err = await pending
  assert.equal(err.name, 'AbortError')
})

test('network failures become a German message', async () => {
  stub(async () => { throw new TypeError('Failed to fetch') })
  const err = await api.status().catch(e => e)
  assert.ok(err instanceof ApiError)
  assert.equal(err.kind, 'network')
  assert.equal(err.message, 'Station nicht erreichbar – neuer Versuch läuft.')
  const save = await api.saveSettings({ a: 'b' }).catch(e => e)
  assert.equal(save.kind, 'network')
  assert.doesNotMatch(save.message, /Failed to fetch/)
})

test('known English backend errors are translated, German ones pass through', async () => {
  stub(async path => path === '/api/relays/x/start' ? json({ error: 'relay id is required' }, 400) : json({ error: 'Benutzername oder Passwort ist falsch' }, 401))
  const relay = await api.startRelay('x').catch(e => e)
  assert.equal(relay.status, 400)
  assert.equal(relay.kind, 'http')
  assert.equal(relay.message, 'Relay-ID fehlt.')
  const login = await api.login({ username: 'admin', password: 'x' }).catch(e => e)
  assert.equal(login.message, 'Benutzername oder Passwort ist falsch')
})

test('401 on a protected call redirects to login, auth calls do not', async () => {
  const redirects: number[] = []
  setUnauthorizedHandler(() => redirects.push(1))
  stub(async () => json({ error: 'login required' }, 401))
  const err = await api.settings().catch(e => e)
  assert.equal(err.message, 'Bitte melde dich an.')
  assert.equal(redirects.length, 1)
  await api.login({ username: 'admin', password: 'x' }).catch(() => undefined)
  await api.authStatus().catch(() => undefined)
  await api.health().catch(() => undefined)
  assert.equal(redirects.length, 1)
})

test('form uploads keep the multipart content type chosen by the browser', async () => {
  stub(async () => json({ path: 'a.tar.gz', files: [], warning: '' }))
  await api.restoreUpload(new File(['x'], 'a.tar.gz'))
  const headers = new Headers(calls[0]!.init.headers)
  assert.equal(headers.get('Content-Type'), null)
  assert.ok(calls[0]!.init.body instanceof FormData)
})

test('concurrent auth status reads share one request and are cached briefly', async () => {
  let release!: () => void
  stub(() => new Promise(resolve => { release = () => resolve(json(auth)) }))
  const all = Promise.all([api.authStatus(), api.authStatus(), api.authStatus()])
  release()
  const results = await all
  assert.equal(calls.length, 1)
  assert.deepEqual(results[2], auth)
  await api.authStatus()
  assert.equal(calls.length, 1)
})

test('auth status cache expires and is dropped on login, logout and failures', async () => {
  mock.timers.enable({ apis: ['Date'], now: 1_000_000 })
  stub(async path => path === '/api/auth/status' ? json(auth) : json({ role: 'admin', status: 'ok' }))
  await api.authStatus()
  mock.timers.tick(5_000)
  await api.authStatus()
  assert.equal(calls.filter(c => c.path === '/api/auth/status').length, 2)
  await api.login({ username: 'admin', password: 'x' })
  await api.authStatus()
  await api.logout()
  await api.authStatus()
  assert.equal(calls.filter(c => c.path === '/api/auth/status').length, 4)

  stub(async () => { throw new TypeError('Failed to fetch') })
  invalidateAuthStatus()
  await assert.rejects(api.authStatus())
  stub(async () => json(auth))
  await api.authStatus()
  assert.equal(calls.length, 1)
})
