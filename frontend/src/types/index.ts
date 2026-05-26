export interface ServiceStatus {
  name: string
  online: boolean
  message?: string
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
}

export interface DeviceCredentials {
  username: string
  stream: string
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
    agentdvr: ServiceStatus
    go2rtc: ServiceStatus
    camera_appliance: ServiceStatus
  }
  slots: Slot[]
  bindings: Binding[]
  devices: Device[]
  recent_events: EventItem[]
  scan_runs: ScanRun[]
}
