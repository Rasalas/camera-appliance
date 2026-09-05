import type { Binding, Device } from '../types'
export function isCameraResource(device: Device, bindings: Binding[]): boolean {
  if (bindings.some(binding => binding.device_id === device.id)) return true
  let raw: Record<string, unknown> = {}
  try { raw = typeof device.raw_json === 'string' ? JSON.parse(device.raw_json) : device.raw_json || {} } catch { return false }
  return !!(raw.manual || raw.rtsp_port_open || raw.onvif_port_open)
}
