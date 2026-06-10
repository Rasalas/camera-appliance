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

export interface WatchdogStatus {
  enabled: boolean
  fast_interval_seconds: number
  camera_interval_seconds: number
  restart_on_change: boolean
  restart_go2rtc_on_failure: boolean
  path_fail_threshold: number
  path_recovery_threshold: number
  path_restart_cooldown_seconds: number
  path_restart_last_at?: string
  path_restart_pending: boolean
  path_restart_cooldown_until?: string
  last_run_at?: string
  next_run_at?: string
  last_action?: string
  last_error?: string
}

export interface RelayEndpointStatus {
  device_id: string
  slot_id?: string
  label?: string
  local_host?: string
  local_port: string
  bind_host: string
  health_host: string
  target_host: string
  target_port: string
  state: string
  message: string
}

export interface RelayStatus {
  id: string
  name: string
  type: string
  host: string
  bind_host: string
  ssh_target?: string
  auto_start: boolean
  enabled: boolean
  pid?: number
  process_state: string
  message: string
  last_error?: string
  backoff_until?: string
  log_path?: string
  endpoints: RelayEndpointStatus[]
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

export type AuthRole = 'admin' | 'viewer'

export interface AuthStatus {
  enabled: boolean
  authenticated: boolean
  role?: AuthRole
  session_expires_at?: string
  admin_password_set: boolean
  viewer_password_set: boolean
  viewer_public: boolean
  local_admin_bypass: boolean
  session_hours: number
  local_admin_bypass_now?: boolean
}

export interface LoginResult {
  role: AuthRole
  expires_at: string
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
  hd_page_url?: string
}

export type ViewerPerformanceMode = 'quality' | 'balanced' | 'low' | 'diagnostic'

export interface ViewerPerformanceOption {
  id: ViewerPerformanceMode | string
  name: string
  description: string
}

export interface ViewerPerformance {
  mode: ViewerPerformanceMode | string
  name: string
  options: ViewerPerformanceOption[]
}

export interface ViewerStreamStatus {
  alias: string
  configured: boolean
  producers: number
  consumers: number
  has_producer: boolean
  has_consumer: boolean
  error?: string
}

export interface DisplayCrop {
  x: number
  y: number
  width: number
  height: number
}

export interface CameraDisplay {
  rotation: 0 | 90 | 180 | 270 | number
  mirror: boolean
  flip: boolean
  fit_mode: 'cover' | 'contain'
  crop: DisplayCrop
}

export type ViewerLayoutID = 'grid_2x2' | 'four_plus_large' | 'vertical_plus_grid' | 'large_only' | 'custom'
export type ViewerLayoutMode = 'auto' | 'focus_left' | 'focus_middle' | 'focus_right' | ViewerLayoutID

export interface ViewerLayoutCell {
  id: string
  slot_id?: string
  area: string
  size: string
  transform?: string
}

export interface ViewerCustomLayoutCell {
  slot_id: string
  column: number
  row: number
  column_span: number
  row_span: number
}

export interface ViewerCustomLayout {
  columns: number[]
  rows: number[]
  cells: ViewerCustomLayoutCell[]
}

export interface ViewerLayoutOption {
  id: ViewerLayoutID | string
  name: string
  description: string
}

export interface ViewerLayout {
  id: ViewerLayoutID | string
  name: string
  mode: ViewerLayoutMode | string
  focus_slot_id: string
  slot_order: string[]
  split_percent: number
  gap_px: number
  cells: ViewerLayoutCell[]
  custom: ViewerCustomLayout
  mosaic?: string
  options: ViewerLayoutOption[]
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
  success_count: number
  failure_count: number
  last_success_at?: string
  last_failure_at?: string
  selected_since?: string
  last_switch_at?: string
  last_switch_reason?: string
  stability: string
  stability_message: string
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
  stream?: ViewerStreamStatus
  path?: StreamPath
  paths?: StreamPath[]
  display: CameraDisplay
  diagnostics?: ViewerDiagnostic[]
}

export interface ViewerResponse {
  checked_at: string
  go2rtc: ServiceStatus
  generated_config?: string
  stream_count: number
  layout: ViewerLayout
  performance: ViewerPerformance
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
    systemd?: ServiceStatus[]
    docker?: ServiceStatus[]
  }
  version: VersionInfo
  watchdog: WatchdogStatus
  relays: RelayStatus[]
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

export interface UpdateStartResult {
  status: string
  url: string
}
