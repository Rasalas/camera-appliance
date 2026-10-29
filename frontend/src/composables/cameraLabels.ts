import type { Binding, Device, ViewerSlotState } from '../types'
import { cameraConfigurationError, type CameraConfiguration, type CameraConfigurationResult } from '../api/cameraConfiguration.ts'

// Mirrors state.DeviceNameMaxLength in the Go manager.
export const cameraNameMaxLength = 40

type NamedDevice = Pick<Device, 'name' | 'manufacturer' | 'model' | 'last_ip'>

export function cameraHardwareLabel(device: Partial<NamedDevice>): string {
  return [device.manufacturer, device.model].map(part => part?.trim()).filter(Boolean).join(' ')
}

export function cameraName(device: Partial<NamedDevice> = {}): string {
  return device.name?.trim() || cameraHardwareLabel(device) || 'Unbekannte Kamera'
}

// Search detail keeps manufacturer/model findable once a custom name replaces them.
export function cameraSearchDetail(device: Partial<NamedDevice>): string {
  return [device.name?.trim() ? cameraHardwareLabel(device) : '', device.last_ip].filter(Boolean).join(' · ')
}

export function cameraNameError(value: string): string {
  const name = value.trim()
  if ([...name].length > cameraNameMaxLength) return `Der Name darf höchstens ${cameraNameMaxLength} Zeichen lang sein.`
  if (/\p{Cc}/u.test(name)) return 'Der Name darf keine Zeilenumbrüche enthalten.'
  return ''
}

export function countLabel(count: number, singular: string, plural: string): string {
  return `${count} ${count === 1 ? singular : plural}`
}

export function cameraStateLabel(state: ViewerSlotState): string {
  const labels: Record<ViewerSlotState, string> = {
    unassigned: 'leer',
    connecting: 'verbindet',
    online: 'live',
    offline: 'offline',
    credentials_failed: 'Zugangsdaten falsch',
    stream_unavailable: 'Kein Bild'
  }
  return labels[state]
}

export function fitModeLabel(mode: string): string {
  return mode === 'cover' ? 'Ausfüllen' : 'Einpassen'
}

export function streamLabel(stream: string): string {
  if (stream === 'stream2') return 'Standard (empfohlen)'
  if (stream === 'stream1') return 'Hohe Auflösung'
  return stream
}

export interface VisibilityApi {
  configureCamera: (id: string, body: CameraConfiguration) => Promise<CameraConfigurationResult>
}

export interface VisibilityInput {
  deviceId: string
  bindings: Pick<Binding, 'slot_id' | 'device_id'>[]
}

// Toggles live-view visibility. The change is saved even when the video
// service restart fails; that failure is returned so the page can show it.
export async function changeCameraVisibility(api: VisibilityApi, input: VisibilityInput): Promise<{ restartError: string; warnings: string[] }> {
  const existing = input.bindings.find(binding => binding.device_id === input.deviceId)
  const result = await api.configureCamera(input.deviceId, { visible: !existing })
  return { restartError: cameraConfigurationError(result), warnings: result.warnings || [] }
}
