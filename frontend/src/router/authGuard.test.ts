import assert from 'node:assert/strict'
import { test } from 'node:test'
import { authDecision, loginTarget } from './authGuard.ts'
import type { AuthStatus } from '../types/index.ts'

const viewer = { requiresViewer: true }
const admin = { requiresAdmin: true }
const status = (patch: Partial<AuthStatus> = {}): AuthStatus => ({
  enabled: true, authenticated: false, admin_password_set: true, viewer_password_set: true,
  viewer_public: false, local_admin_bypass: false, session_hours: 12, ...patch
})

test('a failed auth check keeps the kiosk on the viewer', () => {
  assert.equal(authDecision(viewer, '/', undefined), true)
  assert.equal(authDecision({}, '/login-free', undefined), true)
})

test('a failed auth check sends admin routes to login with a way back', () => {
  assert.deepEqual(authDecision(admin, '/system/zugriff?x=1', undefined), { path: '/login', query: { next: '/system/zugriff?x=1' } })
})

test('disabled auth opens everything', () => {
  assert.equal(authDecision(admin, '/system', status({ enabled: false })), true)
})

test('admin routes need the admin role', () => {
  assert.deepEqual(authDecision(admin, '/system', status({ role: 'viewer' })), { path: '/login', query: { next: '/system' } })
  assert.equal(authDecision(admin, '/system', status({ role: 'admin' })), true)
})

test('viewer routes are open when public, otherwise need a session', () => {
  assert.equal(authDecision(viewer, '/', status({ viewer_public: true })), true)
  assert.deepEqual(authDecision(viewer, '/', status()), { path: '/login', query: { next: '/' } })
  assert.equal(authDecision(viewer, '/', status({ role: 'viewer' })), true)
})

test('login targets stay on this site and never loop back to login', () => {
  assert.equal(loginTarget('/system'), '/system')
  assert.equal(loginTarget('//evil.example'), undefined)
  assert.equal(loginTarget('https://evil.example'), undefined)
  assert.equal(loginTarget('/login?next=/'), undefined)
  assert.equal(loginTarget(undefined), undefined)
})
