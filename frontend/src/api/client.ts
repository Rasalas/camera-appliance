import type { AuthStatus, Binding, CredentialIdentity, Device, DeviceCredentials, EventItem, FrameResult, LoginResult, ManualDeviceResult, ProbeResult, RelayStatus, ScanRun, Slot, StatusResponse, SupportBundleResult, UpdateStartResult, ViewerResponse } from '../types'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: 'same-origin',
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {})
    }
  })
  if (!response.ok) {
    const body = await response.json().catch(() => ({}))
    throw new Error(body.error ?? 'Anfrage fehlgeschlagen')
  }
  return response.json() as Promise<T>
}

export const api = {
  authStatus: () => request<AuthStatus>('/api/auth/status'),
  login: (body: { username: string; password: string; remember?: boolean }) =>
    request<LoginResult>('/api/auth/login', { method: 'POST', body: JSON.stringify(body) }),
  logout: () => request<{ status: string }>('/api/auth/logout', { method: 'POST', body: JSON.stringify({}) }),
  setAuthPassword: (body: { role: 'admin' | 'viewer'; password: string }) =>
    request<{ status: string }>('/api/auth/password', { method: 'POST', body: JSON.stringify(body) }),
  status: () => request<StatusResponse>('/api/status'),
  viewer: () => request<ViewerResponse>('/api/viewer'),
  discover: () => request<{ devices: Device[]; subnets: Array<{ cidr: string; interface: string }> }>('/api/discovery/start', { method: 'POST' }),
  runs: () => request<ScanRun[]>('/api/discovery/runs'),
  devices: () => request<Device[]>('/api/devices'),
  device: (id: string) => request<Device>(`/api/devices/${id}`),
  addManualDevice: (body: { ip: string; username: string; password?: string; stream: string; label?: string }) =>
    request<ManualDeviceResult>('/api/devices/manual', { method: 'POST', body: JSON.stringify(body) }),
  probeDevice: (id: string, body: { username: string; password: string; stream: string }) =>
    request<ProbeResult>(`/api/devices/${id}/probe`, { method: 'POST', body: JSON.stringify(body) }),
  captureFrame: (id: string, body: { username: string; password: string; stream: string; save?: boolean }) =>
    request<FrameResult>(`/api/devices/${id}/frame`, { method: 'POST', body: JSON.stringify(body) }),
  referenceImageUrl: (id: string, revision = 0) => `/api/devices/${encodeURIComponent(id)}/reference-image?v=${revision}`,
  deviceCredentials: (id: string) => request<DeviceCredentials>(`/api/devices/${id}/credentials`),
  saveDeviceCredentials: (id: string, body: { username: string; password?: string; stream: string }) =>
    request<DeviceCredentials>(`/api/devices/${id}/credentials`, { method: 'POST', body: JSON.stringify(body) }),
  slots: () => request<Slot[]>('/api/slots'),
  bindings: () => request<Binding[]>('/api/bindings'),
  saveBinding: (binding: Partial<Binding>) => request('/api/bindings', { method: 'POST', body: JSON.stringify(binding) }),
  removeBinding: (slotId: string) => request(`/api/bindings/${slotId}`, { method: 'DELETE' }),
  renderGo2RTC: () => request<{ rendered_streams: number; warnings: string[]; redacted_yaml: string }>('/api/go2rtc/render', { method: 'POST' }),
  restartGo2RTC: () => request('/api/go2rtc/restart', { method: 'POST' }),
  relayStatus: () => request<RelayStatus[]>('/api/relays/status'),
  startRelay: (id: string) => request<RelayStatus>(`/api/relays/${encodeURIComponent(id)}/start`, { method: 'POST' }),
  stopRelay: (id: string) => request<RelayStatus>(`/api/relays/${encodeURIComponent(id)}/stop`, { method: 'POST' }),
  restartRelay: (id: string) => request<RelayStatus>(`/api/relays/${encodeURIComponent(id)}/restart`, { method: 'POST' }),
  restartStack: () => request('/api/system/restart-stack', { method: 'POST' }),
  startUpdate: (url?: string) => request<UpdateStartResult>('/api/system/update', { method: 'POST', body: JSON.stringify({ url: url || '' }) }),
  credentialIdentities: () => request<CredentialIdentity[]>('/api/credential-identities'),
  saveCredentialIdentity: (body: { id?: string; name: string; username: string; password?: string; copy_password_from_id?: string }) =>
    request<CredentialIdentity>('/api/credential-identities', { method: 'POST', body: JSON.stringify(body) }),
  deleteCredentialIdentity: (id: string) => request(`/api/credential-identities/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  settings: () => request<Record<string, string>>('/api/settings'),
  saveSettings: (settings: Record<string, string>) => request('/api/settings', { method: 'PUT', body: JSON.stringify(settings) }),
  saveCameraPassword: (password: string) => request<{ status: string; source: string }>('/api/secrets/camera-password', { method: 'POST', body: JSON.stringify({ password }) }),
  events: () => request<EventItem[]>('/api/events'),
  backup: () => request<{ path: string; files: string[]; warning: string }>('/api/backup', { method: 'POST', body: JSON.stringify({}) }),
  supportBundle: () => request<SupportBundleResult>('/api/support-bundle', { method: 'POST', body: JSON.stringify({}) }),
  restore: (path: string) => request<{ path: string; files: string[]; warning: string }>('/api/restore', { method: 'POST', body: JSON.stringify({ in: path }) })
}
