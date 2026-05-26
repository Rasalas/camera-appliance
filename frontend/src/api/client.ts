import type { Binding, Device, EventItem, FrameResult, ProbeResult, ScanRun, Slot, StatusResponse } from '../types'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
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
  status: () => request<StatusResponse>('/api/status'),
  discover: () => request<{ devices: Device[]; subnets: Array<{ cidr: string; interface: string }> }>('/api/discovery/start', { method: 'POST' }),
  runs: () => request<ScanRun[]>('/api/discovery/runs'),
  devices: () => request<Device[]>('/api/devices'),
  device: (id: string) => request<Device>(`/api/devices/${id}`),
  probeDevice: (id: string, body: { username: string; password: string; stream: string }) =>
    request<ProbeResult>(`/api/devices/${id}/probe`, { method: 'POST', body: JSON.stringify(body) }),
  captureFrame: (id: string, body: { username: string; password: string; stream: string; save?: boolean }) =>
    request<FrameResult>(`/api/devices/${id}/frame`, { method: 'POST', body: JSON.stringify(body) }),
  slots: () => request<Slot[]>('/api/slots'),
  bindings: () => request<Binding[]>('/api/bindings'),
  saveBinding: (binding: Partial<Binding>) => request('/api/bindings', { method: 'POST', body: JSON.stringify(binding) }),
  removeBinding: (slotId: string) => request(`/api/bindings/${slotId}`, { method: 'DELETE' }),
  renderGo2RTC: () => request<{ rendered_streams: number; warnings: string[]; redacted_yaml: string }>('/api/go2rtc/render', { method: 'POST' }),
  restartGo2RTC: () => request('/api/go2rtc/restart', { method: 'POST' }),
  restartStack: () => request('/api/system/restart-stack', { method: 'POST' }),
  settings: () => request<Record<string, string>>('/api/settings'),
  saveSettings: (settings: Record<string, string>) => request('/api/settings', { method: 'PUT', body: JSON.stringify(settings) }),
  events: () => request<EventItem[]>('/api/events'),
  backup: () => request<{ path: string; files: string[]; warning: string }>('/api/backup', { method: 'POST', body: JSON.stringify({}) }),
  restore: (path: string) => request<{ path: string; files: string[]; warning: string }>('/api/restore', { method: 'POST', body: JSON.stringify({ in: path }) })
}
