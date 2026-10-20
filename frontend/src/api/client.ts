import type { AuthStatus, Binding, CredentialIdentity, Device, DeviceCredentials, EventItem, FrameResult, LoginResult, ManualDeviceResult, ProbeResult, RelayStatus, ScanRun, Slot, StatusResponse, SupportBundleResult, UpdateStartResult, UpdateFlowStatus, ViewerResponse } from '../types'

import type { UploadSettings, UploadCrop, SnapshotUploadResult, UploadScheduleInput, UploadScheduleStatus, UploadNaming, UploadImageSettings } from '../types'

export type BackupResult = { path: string; files: string[]; warning: string }

export type ApiErrorKind = 'http' | 'network' | 'timeout'

// Callers rely on `err instanceof Error` and `err.message`; keep that shape.
export class ApiError extends Error {
  kind: ApiErrorKind
  status: number
  constructor(message: string, kind: ApiErrorKind, status = 0) {
    super(message)
    this.name = 'ApiError'
    this.kind = kind
    this.status = status
  }
}

export type RequestOptions = { timeoutMs?: number; redirectOn401?: boolean }

export const DEFAULT_TIMEOUT_MS = 10_000
const SLOW = { timeoutMs: 60_000 }
const VERY_SLOW = { timeoutMs: 180_000 }
const PUBLIC = { redirectOn401: false }

// Exact backend messages that can reach users in English.
const backendMessages: Record<string, string> = {
  'login required': 'Bitte melde dich an.',
  'admin login required': 'Dafür ist eine Admin-Anmeldung nötig.',
  'not found': 'Nicht gefunden.',
  'scan run not found': 'Suchlauf nicht gefunden.',
  'password is required': 'Bitte ein Passwort eingeben.',
  'name is required': 'Bitte einen Namen eingeben.',
  'username is required': 'Bitte einen Benutzernamen eingeben.',
  'relay id is required': 'Relay-ID fehlt.',
  'update url is invalid': 'Die Update-Adresse ist ungültig.',
  'input backup path is required': 'Bitte eine Sicherungsdatei angeben.',
  'support bundle did not include any files': 'Das Diagnosepaket enthält keine Dateien.',
  'cross-origin request abgelehnt': 'Anfrage abgelehnt: Sie kam nicht von dieser Station.',
  'http: request body too large': 'Die Datei ist zu groß.',
  'unexpected eof': 'Die Anfrage war unvollständig.',
  'eof': 'Die Anfrage war unvollständig.'
}

export function germanError(message: string, status: number): string {
  const known = backendMessages[message.trim().toLowerCase()]
  if (known && status === 403 && message.trim().toLowerCase() === 'login required') return backendMessages['admin login required']!
  return known ?? message
}

function transportError(kind: 'network' | 'timeout', method: string): ApiError {
  // Reads are retried by polling pages; actions are not.
  const read = method === 'GET'
  if (kind === 'timeout') {
    return new ApiError(read ? 'Station antwortet nicht – neuer Versuch läuft.' : 'Station antwortet nicht rechtzeitig. Bitte prüfen, ob die Änderung übernommen wurde.', kind)
  }
  return new ApiError(read ? 'Station nicht erreichbar – neuer Versuch läuft.' : 'Station nicht erreichbar. Bitte gleich noch einmal versuchen.', kind)
}

let unauthorizedHandler: (() => void) | undefined
export function setUnauthorizedHandler(handler: (() => void) | undefined) { unauthorizedHandler = handler }

// A manual controller works in every browser and lets us clear the timer once the body is read.
async function send<R>(path: string, init: RequestInit | undefined, options: RequestOptions, read: (response: Response) => Promise<R>): Promise<R> {
  const method = (init?.method ?? 'GET').toUpperCase()
  const controller = new AbortController()
  const caller = init?.signal
  const forward = () => controller.abort(caller?.reason)
  if (caller?.aborted) forward()
  else caller?.addEventListener('abort', forward, { once: true })
  let timedOut = false
  const timer = setTimeout(() => { timedOut = true; controller.abort() }, options.timeoutMs ?? DEFAULT_TIMEOUT_MS)
  const headers = new Headers(init?.headers)
  if (!(init?.body instanceof FormData) && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  try {
    let response: Response
    try {
      response = await fetch(path, { ...init, credentials: 'same-origin', headers, signal: controller.signal })
    } catch (err) {
      if (caller?.aborted) throw err
      throw transportError(timedOut ? 'timeout' : 'network', method)
    }
    if (!response.ok) {
      const body = await response.json().catch(() => ({}))
      if (response.status === 401 && options.redirectOn401 !== false) {
        invalidateAuthStatus()
        unauthorizedHandler?.()
      }
      const message = typeof body.error === 'string' && body.error ? body.error : 'Anfrage fehlgeschlagen'
      throw new ApiError(germanError(message, response.status), 'http', response.status)
    }
    try {
      const result = await read(response)
      if (method !== 'GET') invalidateAuthStatus()
      return result
    } catch (err) {
      if (caller?.aborted || !timedOut) throw err
      throw transportError('timeout', method)
    }
  } finally {
    clearTimeout(timer)
    caller?.removeEventListener('abort', forward)
  }
}

export function request<T>(path: string, init?: RequestInit, options: RequestOptions = {}): Promise<T> {
  return send(path, init, options, response => response.json() as Promise<T>)
}

// Auth status is read by the router guard, layout and pages on every navigation.
const AUTH_STATUS_TTL_MS = 3_000
let authCache: { value: AuthStatus; at: number } | undefined
let authPending: Promise<AuthStatus> | undefined
let authGeneration = 0

export function invalidateAuthStatus() {
  authGeneration++
  authCache = undefined
  authPending = undefined
}

function authStatus(): Promise<AuthStatus> {
  if (authCache && Date.now() - authCache.at < AUTH_STATUS_TTL_MS) return Promise.resolve(authCache.value)
  if (authPending) return authPending
  const generation = authGeneration
  const pending = request<AuthStatus>('/api/auth/status', undefined, PUBLIC).then(value => {
    if (generation === authGeneration) authCache = { value, at: Date.now() }
    return value
  }).finally(() => {
    if (authPending === pending) authPending = undefined
  })
  authPending = pending
  return pending
}

export const api = {
  health: (options: RequestOptions = {}) => request<{ version: string; commit: string; status: string }>('/api/health', undefined, { ...options, ...PUBLIC }),
  uploadSchedule: (id: string) => request<UploadScheduleStatus>(`/api/devices/${encodeURIComponent(id)}/upload-schedule`),
  uploadImageSettings: (id: string) => request<UploadImageSettings>(`/api/devices/${encodeURIComponent(id)}/upload-image-settings`),
  saveUploadImageSettings: (id: string, body: UploadImageSettings) => request<UploadImageSettings>(`/api/devices/${encodeURIComponent(id)}/upload-image-settings`, { method: 'PUT', body: JSON.stringify(body) }),
  uploadNaming: (id: string) => request<UploadNaming>(`/api/devices/${encodeURIComponent(id)}/upload-naming`),
  saveUploadNaming: (id: string, body: UploadNaming) => request<UploadNaming>(`/api/devices/${encodeURIComponent(id)}/upload-naming`, { method: 'PUT', body: JSON.stringify(body) }),
  saveUploadSchedule: (id: string, body: UploadScheduleInput) => request<UploadScheduleStatus>(`/api/devices/${encodeURIComponent(id)}/upload-schedule`, { method: 'PUT', body: JSON.stringify(body) }),
  uploadSettings: () => request<UploadSettings>('/api/snapshot-upload'),
  saveUploadSettings: (body: Omit<UploadSettings, 'password_set'> & { password: string; clear_password?: boolean }) =>
    request<UploadSettings>('/api/snapshot-upload', { method: 'PUT', body: JSON.stringify(body) }),
  uploadCrop: (id: string) => request<UploadCrop>(`/api/devices/${encodeURIComponent(id)}/upload-crop`),
  saveUploadCrop: (id: string, crop: UploadCrop) => request<UploadCrop>(`/api/devices/${encodeURIComponent(id)}/upload-crop`, { method: 'PUT', body: JSON.stringify(crop) }),
  uploadSnapshot: (id: string, body: { username: string; password: string; stream: string; crop: UploadCrop }) =>
    request<SnapshotUploadResult>(`/api/devices/${encodeURIComponent(id)}/upload-snapshot`, { method: 'POST', body: JSON.stringify(body) }, SLOW),
  authStatus,
  login: (body: { username: string; password: string; remember?: boolean }) =>
    request<LoginResult>('/api/auth/login', { method: 'POST', body: JSON.stringify(body) }, PUBLIC),
  logout: () => request<{ status: string }>('/api/auth/logout', { method: 'POST', body: JSON.stringify({}) }, PUBLIC),
  setAuthPassword: (body: { role: 'admin' | 'viewer'; password: string }) =>
    request<{ status: string }>('/api/auth/password', { method: 'POST', body: JSON.stringify(body) }, PUBLIC),
  status: () => request<StatusResponse>('/api/status'),
  viewer: () => request<ViewerResponse>('/api/viewer'),
  discover: () => request<{ devices: Device[]; subnets: Array<{ cidr: string; interface: string }> }>('/api/discovery/start', { method: 'POST' }, { timeoutMs: 90_000 }),
  runs: () => request<ScanRun[]>('/api/discovery/runs'),
  devices: () => request<Device[]>('/api/devices'),
  device: (id: string) => request<Device>(`/api/devices/${id}`),
  saveDeviceName: (id: string, name: string) => request<Device>(`/api/devices/${encodeURIComponent(id)}/name`, { method: 'PUT', body: JSON.stringify({ name }) }),
  addManualDevice: (body: { ip: string; username: string; password?: string; stream: string; label?: string }) =>
    request<ManualDeviceResult>('/api/devices/manual', { method: 'POST', body: JSON.stringify(body) }, SLOW),
  probeDevice: (id: string, body: { username: string; password: string; stream: string }) =>
    request<ProbeResult>(`/api/devices/${id}/probe`, { method: 'POST', body: JSON.stringify(body) }, SLOW),
  captureFrame: (id: string, body: { username: string; password: string; stream: string; save?: boolean }) =>
    request<FrameResult>(`/api/devices/${id}/frame`, { method: 'POST', body: JSON.stringify(body) }, SLOW),
  referenceImageUrl: (id: string, revision = 0) => `/api/devices/${encodeURIComponent(id)}/reference-image?v=${revision}`,
  deviceCredentials: (id: string) => request<DeviceCredentials>(`/api/devices/${id}/credentials`),
  saveDeviceCredentials: (id: string, body: { username: string; password?: string; stream: string }) =>
    request<DeviceCredentials>(`/api/devices/${id}/credentials`, { method: 'POST', body: JSON.stringify(body) }),
  slots: () => request<Slot[]>('/api/slots'),
  bindings: () => request<Binding[]>('/api/bindings'),
  saveBinding: (binding: Partial<Binding>) => request('/api/bindings', { method: 'POST', body: JSON.stringify(binding) }),
  removeBinding: (slotId: string) => request(`/api/bindings/${slotId}`, { method: 'DELETE' }),
  renderGo2RTC: () => request<{ rendered_streams: number; warnings: string[]; redacted_yaml: string }>('/api/go2rtc/render', { method: 'POST' }),
  restartGo2RTC: () => request('/api/go2rtc/restart', { method: 'POST' }, SLOW),
  relayStatus: () => request<RelayStatus[]>('/api/relays/status'),
  startRelay: (id: string) => request<RelayStatus>(`/api/relays/${encodeURIComponent(id)}/start`, { method: 'POST' }, SLOW),
  stopRelay: (id: string) => request<RelayStatus>(`/api/relays/${encodeURIComponent(id)}/stop`, { method: 'POST' }),
  restartRelay: (id: string) => request<RelayStatus>(`/api/relays/${encodeURIComponent(id)}/restart`, { method: 'POST' }, SLOW),
  restartStack: () => request('/api/system/restart-stack', { method: 'POST' }, VERY_SLOW),
  startUpdate: (url?: string, digest?: string) => request<UpdateStartResult>('/api/system/update', { method: 'POST', body: JSON.stringify({ url: url || '', digest: digest || '' }) }, SLOW),
  getUpdateStatus: () => request<UpdateFlowStatus>('/api/system/update/status'),
  checkForUpdates: () => request<UpdateFlowStatus>('/api/system/update/check', { method: 'POST', body: JSON.stringify({}) }, SLOW),
  downloadUpdate: () => request<UpdateFlowStatus>('/api/system/update/download', { method: 'POST', body: JSON.stringify({}) }, SLOW),
  installUpdate: () => request<{ status: string }>('/api/system/update/install', { method: 'POST', body: JSON.stringify({}) }, SLOW),
  credentialIdentities: () => request<CredentialIdentity[]>('/api/credential-identities'),
  saveCredentialIdentity: (body: { id?: string; name: string; username: string; password?: string; copy_password_from_id?: string }) =>
    request<CredentialIdentity>('/api/credential-identities', { method: 'POST', body: JSON.stringify(body) }),
  deleteCredentialIdentity: (id: string) => request(`/api/credential-identities/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  settings: () => request<Record<string, string>>('/api/settings'),
  saveSettings: (settings: Record<string, string>) => request('/api/settings', { method: 'PUT', body: JSON.stringify(settings) }),
  saveCameraPassword: (password: string) => request<{ status: string; source: string }>('/api/secrets/camera-password', { method: 'POST', body: JSON.stringify({ password }) }),
  events: () => request<EventItem[]>('/api/events'),
  backup: () => request<BackupResult>('/api/backup', { method: 'POST', body: JSON.stringify({}) }, VERY_SLOW),
  supportBundle: () => request<SupportBundleResult>('/api/support-bundle', { method: 'POST', body: JSON.stringify({}) }, VERY_SLOW),
  supportReport: () => request<{ version: { version: string; commit: string }; events: EventItem[] }>('/api/support'),
  downloadSupportBundle: () => send('/api/support-bundle/download', { method: 'POST', body: '{}' }, VERY_SLOW, response => response.blob()),
  restore: (path: string) => request<BackupResult>('/api/restore', { method: 'POST', body: JSON.stringify({ in: path }) }, VERY_SLOW),
  restoreUpload: (file: File) => {
    const body = new FormData()
    body.append('backup', file)
    return request<BackupResult>('/api/restore', { method: 'POST', body }, VERY_SLOW)
  }
}
