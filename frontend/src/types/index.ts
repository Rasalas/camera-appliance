export interface ServiceStatus {
  name: string
  online: boolean
  message?: string
}

export interface VersionInfo {
  version: string
  commit: string
  build_time: string
}

export interface Slot {
  id: string
  label: string
  role: string
  default_stream: string
  required: boolean
  sort_order: number
}

export interface Device {
  id: string
  first_seen_at: string
  last_seen_at: string
  last_ip?: string
  mac_address?: string
  onvif_endpoint_ref?: string
  serial_number?: string
  manufacturer?: string
  model?: string
  hardware_id?: string
  hostname?: string
  raw_json?: Record<string, unknown> | string
}

export interface ProbeResult {
  success: boolean
  url_redacted: string
  message: string
}

export interface FrameResult {
  content_type: string
  image_base64: string
  sha256: string
  url_redacted: string
  saved_path?: string
  credential_source?: string
  identity_id?: string
}

export interface DeviceCredentials {
  username: string
  stream: string
  password_set: boolean
  password_source?: string
}

export interface CredentialIdentity {
  id: string
  name: string
  username: string
  password_set: boolean
  password_source?: string
}

export interface ManualDeviceResult {
  device: Device
  rtsp_port_open: boolean
  message: string
}

export interface Binding {
  slot_id: string
  device_id: string
  label?: string
  username?: string
  stream_name: string
  enabled: boolean
  device?: Device
  slot?: Slot
}

export type ViewerSlotState =
  | 'unassigned'
  | 'connecting'
  | 'online'
  | 'offline'
  | 'credentials_failed'
  | 'stream_unavailable'

export interface ViewerDiagnostic {
  key: string
  status: string
  message: string
}

export interface ViewerPlayback {
  page_url: string
}

export interface StreamPath {
  id: string
  label: string
  kind: 'direct' | 'relay' | string
  relay_id?: string
  host: string
  port: string
  probe_host?: string
  state: string
  message: string
  active: boolean
  selected: boolean
  last_selected: boolean
}

export interface ViewerSlot {
  slot: Slot
  alias: string
  label: string
  state: ViewerSlotState
  message: string
  binding?: Binding
  device?: Device
  playback?: ViewerPlayback
  path?: StreamPath
  paths?: StreamPath[]
  diagnostics?: ViewerDiagnostic[]
}

export interface ViewerResponse {
  checked_at: string
  go2rtc: ServiceStatus
  generated_config?: string
  stream_count: number
  slots: ViewerSlot[]
}

export interface EventItem {
  id: string
  created_at: string
  level: string
  type: string
  message: string
}

export interface ScanRun {
  id: string
  started_at: string
  finished_at?: string
  status: string
  message?: string
}

export interface StatusResponse {
  system: {
    go2rtc: ServiceStatus
    camera_appliance: ServiceStatus
  }
  version: VersionInfo
  slots: Slot[]
  bindings: Binding[]
  devices: Device[]
  recent_events: EventItem[]
  scan_runs: ScanRun[]
}

export interface SupportBundleResult {
  path: string
  files: string[]
  warning: string
}
