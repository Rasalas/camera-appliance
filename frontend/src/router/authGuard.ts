import type { AuthStatus } from '../types/index.ts'

export type GuardMeta = { requiresAdmin?: unknown; requiresViewer?: unknown }
export type LoginRedirect = { path: '/login'; query: { next: string } }

// `auth` is undefined when the status check itself failed (network or 5xx).
export function authDecision(meta: GuardMeta, fullPath: string, auth: AuthStatus | undefined): true | LoginRedirect {
  const login: LoginRedirect = { path: '/login', query: { next: fullPath } }
  if (!auth) {
    // The backend still enforces viewer access; keep a kiosk on its cameras during restarts.
    return meta.requiresAdmin ? login : true
  }
  if (!auth.enabled) return true
  if (meta.requiresAdmin && auth.role !== 'admin') return login
  if (meta.requiresViewer && !auth.viewer_public && auth.role !== 'admin' && auth.role !== 'viewer') return login
  return true
}

export function loginTarget(next: unknown): string | undefined {
  if (typeof next !== 'string' || !next.startsWith('/') || next.startsWith('//') || next.startsWith('/login')) return undefined
  return next
}
