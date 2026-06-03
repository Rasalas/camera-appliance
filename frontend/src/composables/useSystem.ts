import { computed, reactive, ref } from 'vue'
import { api } from '../api/client'
import type {
  AuthRole,
  AuthStatus,
  CredentialIdentity,
  EventItem,
  RelayStatus,
  StatusResponse,
  SupportBundleResult,
  ViewerPerformanceOption
} from '../types'

// Shared System state — loaded once and reused across the System subpages.
// Settings are an upsert-per-key store on the backend (PutSettings), so each page
// may save just its own keys without clobbering the others.

const viewerPerformanceOptions: ViewerPerformanceOption[] = [
  { id: 'quality', name: 'Qualität', description: 'Alle sichtbaren Streams sofort live laden.' },
  { id: 'balanced', name: 'Balanciert', description: 'Nebenansichten lazy laden und primäre Ansicht priorisieren.' },
  { id: 'low', name: 'Niedrig', description: 'Nur die primäre Ansicht live laden, Nebenansichten pausieren.' },
  { id: 'diagnostic', name: 'Diagnose', description: 'Alle Streams live laden und Producer/Consumer sichtbar machen.' }
]

const settings = reactive<Record<string, string>>({})
const status = ref<StatusResponse>()
const authStatus = ref<AuthStatus>()
const events = ref<EventItem[]>([])
const credentialIdentities = ref<CredentialIdentity[]>([])
const error = ref('')
const toast = ref('')
const passwordSource = ref('unbekannt')
let loaded = false

const relayIds = computed(() => settingList(settings['camera.relay.ids']))
const relayStatuses = computed(() => status.value?.relays ?? [])
const cameraBindings = computed(() => (status.value?.bindings ?? []).filter((binding) => binding.device_id))
const watchdogEnabled = computed(() => boolSetting('watchdog.enabled', status.value?.watchdog?.enabled ?? true))
const watchdogRestartOnChange = computed(() => boolSetting('watchdog.restart_on_change', status.value?.watchdog?.restart_on_change ?? true))
const watchdogRestartGo2RTC = computed(() => boolSetting('watchdog.restart_go2rtc_on_failure', status.value?.watchdog?.restart_go2rtc_on_failure ?? true))
const restartCooldownLabel = computed(() => {
  const watchdog = status.value?.watchdog
  if (!watchdog) return 'Noch kein Status.'
  if (watchdog.path_restart_pending) return `Ausstehend bis ${watchdogDate(watchdog.path_restart_cooldown_until)}`
  if (watchdog.path_restart_last_at) return `Letzter Restart ${watchdogDate(watchdog.path_restart_last_at)}`
  return 'Kein Cooldown aktiv.'
})
const versionLabel = computed(() => {
  const info = status.value?.version
  if (!info) return 'dev'
  const version = info.version || 'dev'
  const commit = info.commit && info.commit !== 'local' ? ` (${info.commit})` : ''
  return `${version}${commit}`
})
const versionDetail = computed(() => {
  const info = status.value?.version
  if (!info) return 'dev'
  const parts = [info.version || 'dev']
  if (info.commit) parts.push(`Commit ${info.commit}`)
  if (info.build_time) parts.push(`Build ${info.build_time}`)
  return parts.join(' · ')
})
const viewerPerformanceDescription = computed(() => {
  const mode = normalizedViewerPerformanceMode(settings['viewer.performance.mode'])
  return viewerPerformanceOptions.find((option) => option.id === mode)?.description || ''
})

function showToast(message: string) {
  toast.value = message
  setTimeout(() => (toast.value = ''), 2200)
}

function setBool(key: string, e: Event) {
  settings[key] = (e.target as HTMLInputElement).checked ? 'true' : 'false'
}

function boolSetting(key: string, fallback: boolean) {
  const raw = settings[key]
  if (raw === undefined || raw === '') return fallback
  return raw === 'true' || raw === '1' || raw === 'yes' || raw === 'on'
}

function formatTime(t: string) {
  return new Date(t).toLocaleString('de-DE', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function watchdogDate(value?: string) {
  if (!value) return 'Noch nicht gelaufen.'
  return formatTime(value)
}

function levelClass(l: string) {
  const lower = (l || '').toLowerCase()
  if (lower.includes('err') || lower.includes('fail')) return 'err'
  if (lower.includes('warn')) return 'warn'
  if (lower.includes('ok') || lower.includes('info')) return 'ok'
  return ''
}

function passwordSourceLabel(source?: string) {
  if (!source) return 'Passwort gespeichert'
  if (source === 'keyring') return 'Keyring'
  if (source === 'local.env') return 'Secret-Datei'
  return source
}

function normalizedViewerPerformanceMode(raw?: string): 'quality' | 'balanced' | 'low' | 'diagnostic' {
  if (raw === 'balanced' || raw === 'low' || raw === 'diagnostic') return raw
  return 'quality'
}

function settingList(raw?: string) {
  const seen = new Set<string>()
  return (raw || '')
    .split(',')
    .map((part) => part.trim())
    .filter((part) => {
      if (!part || seen.has(part)) return false
      seen.add(part)
      return true
    })
}

function sanitizeID(value: string) {
  return value.trim().toLowerCase().replace(/[^a-z0-9_]+/g, '_').replace(/^_+|_+$/g, '')
}

function relaySettingKey(id: string, field: string) {
  return `camera.relay.${id}.${field}`
}
function relayEndpointKey(deviceId: string, relayId: string, field: string) {
  return `camera.relay_endpoint.${deviceId}.${relayId}.${field}`
}
function pathPolicyKey(deviceId: string) {
  return `camera.path_policy.${deviceId}`
}
function relayName(id: string) {
  return settings[relaySettingKey(id, 'name')] || id
}
function relayHost(id: string) {
  return settings[relaySettingKey(id, 'host')] || ''
}
function relayAutoStart(id: string) {
  return boolSetting(relaySettingKey(id, 'auto_start'), relayStatusFor(id)?.auto_start ?? false)
}
function relayStatusFor(id: string): RelayStatus | undefined {
  return relayStatuses.value.find((relay) => relay.id === id)
}
function relayStateLabel(id: string) {
  const state = relayStatusFor(id)?.process_state || 'unknown'
  const labels: Record<string, string> = {
    running: 'Läuft', stopped: 'Gestoppt', stale: 'Prozess beendet', unmanaged: 'Manuell',
    external: 'Externer Forward', backoff: 'Backoff', error: 'Fehler', not_configured: 'Unvollständig',
    unsupported: 'Nicht unterstützt', disabled: 'Deaktiviert', unknown: 'Unbekannt'
  }
  return labels[state] || state
}
function relayStateClass(id: string) {
  const state = relayStatusFor(id)?.process_state
  if (state === 'running' || state === 'external') return 'ok'
  if (state === 'error' || state === 'stale' || state === 'not_configured') return 'err'
  if (state === 'backoff' || state === 'stopped') return 'warn'
  return ''
}
function relayEndpointStatus(deviceId: string, relayId: string) {
  return relayStatusFor(relayId)?.endpoints.find((endpoint) => endpoint.device_id === deviceId)
}
function relayEndpointStateLabel(deviceId: string, relayId: string) {
  const endpoint = relayEndpointStatus(deviceId, relayId)
  if (!endpoint) return 'kein Status'
  if (endpoint.state === 'ok') return 'Port ok'
  if (endpoint.state === 'failed') return 'Port offline'
  return 'unvollständig'
}
function relayEndpointStateClass(deviceId: string, relayId: string) {
  const state = relayEndpointStatus(deviceId, relayId)?.state
  if (state === 'ok') return 'ok'
  if (state === 'failed') return 'err'
  return 'warn'
}
function legacyRelayHost(deviceId: string) {
  return settings[`camera.rtsp_endpoint.${deviceId}.host`] || ''
}
function legacyRelayPort(deviceId: string) {
  return settings[`camera.rtsp_endpoint.${deviceId}.port`] || '554'
}

function ensurePathPolicyDefaults() {
  for (const binding of cameraBindings.value) {
    const key = pathPolicyKey(binding.device_id)
    if (!settings[key]) settings[key] = 'auto'
  }
}
function ensureWatchdogDefaults() {
  const watchdog = status.value?.watchdog
  if (!watchdog) return
  if (!settings['watchdog.enabled']) settings['watchdog.enabled'] = String(watchdog.enabled)
  if (!settings['watchdog.restart_on_change']) settings['watchdog.restart_on_change'] = String(watchdog.restart_on_change)
  if (!settings['watchdog.restart_go2rtc_on_failure']) settings['watchdog.restart_go2rtc_on_failure'] = String(watchdog.restart_go2rtc_on_failure)
  if (!settings['watchdog.fast_interval_seconds']) settings['watchdog.fast_interval_seconds'] = String(watchdog.fast_interval_seconds)
  if (!settings['watchdog.camera_interval_seconds']) settings['watchdog.camera_interval_seconds'] = String(watchdog.camera_interval_seconds)
  if (!settings['camera.path.fail_threshold']) settings['camera.path.fail_threshold'] = String(watchdog.path_fail_threshold)
  if (!settings['camera.path.recovery_threshold']) settings['camera.path.recovery_threshold'] = String(watchdog.path_recovery_threshold)
  if (!settings['camera.path.restart_cooldown_seconds']) settings['camera.path.restart_cooldown_seconds'] = String(watchdog.path_restart_cooldown_seconds)
}
function ensureAuthDefaults() {
  const auth = authStatus.value
  if (!auth) return
  settings.auth_admin_password_set = String(auth.admin_password_set)
  settings.auth_viewer_password_set = String(auth.viewer_password_set)
  if (!settings['auth.viewer_public']) settings['auth.viewer_public'] = String(auth.viewer_public)
  if (!settings['auth.local_admin_bypass']) settings['auth.local_admin_bypass'] = String(auth.local_admin_bypass)
  if (!settings['auth.session_hours']) settings['auth.session_hours'] = String(auth.session_hours || 12)
}
function ensureRelayDefaults() {
  for (const relayId of relayIds.value) {
    if (!settings[relaySettingKey(relayId, 'type')]) settings[relaySettingKey(relayId, 'type')] = 'ssh_local_forward'
    if (!settings[relaySettingKey(relayId, 'bind_host')]) settings[relaySettingKey(relayId, 'bind_host')] = relayStatusFor(relayId)?.bind_host || '127.0.0.1'
    if (!settings[relaySettingKey(relayId, 'auto_start')]) settings[relaySettingKey(relayId, 'auto_start')] = String(relayStatusFor(relayId)?.auto_start ?? false)
  }
}
function ensurePerformanceDefault() {
  if (!settings['viewer.performance.mode']) settings['viewer.performance.mode'] = 'quality'
}

async function loadAll(force = false) {
  if (loaded && !force) return
  try {
    Object.assign(settings, await api.settings())
    passwordSource.value = settings.camera_password_source === 'keyring' ? 'Betriebssystem-Keyring' : (settings.camera_password_source || 'unbekannt')
    authStatus.value = await api.authStatus()
    ensureAuthDefaults()
    await refreshStatus()
    credentialIdentities.value = await api.credentialIdentities()
    events.value = await api.events()
    ensurePerformanceDefault()
    loaded = true
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Konnte nicht geladen werden.'
  }
}

async function refreshStatus() {
  status.value = await api.status()
  ensurePathPolicyDefaults()
  ensureWatchdogDefaults()
  ensureRelayDefaults()
}

async function saveSettings(keys?: string[]) {
  error.value = ''
  try {
    const payload = keys
      ? Object.fromEntries(keys.filter((k) => k in settings).map((k) => [k, settings[k]]))
      : { ...settings }
    await api.saveSettings(payload)
    showToast('Einstellungen gespeichert')
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Speichern fehlgeschlagen.'
  }
}

const backupResult = ref<{ path: string; warning: string }>()
const supportBundleResult = ref<SupportBundleResult>()

async function saveCameraPassword(password: string) {
  error.value = ''
  const result = await api.saveCameraPassword(password)
  settings.camera_password_set = 'true'
  settings.camera_password_source = result.source
  passwordSource.value = result.source === 'keyring' ? 'Betriebssystem-Keyring' : result.source
  showToast('Kamera-Passwort gespeichert')
}

async function saveAuthPassword(role: AuthRole, password: string) {
  const wasEnabled = authStatus.value?.enabled ?? false
  error.value = ''
  await api.setAuthPassword({ role, password })
  if (role === 'admin' && !wasEnabled) {
    await api.login({ username: 'admin', password })
    window.dispatchEvent(new Event('auth-changed'))
  }
  if (role === 'admin') settings.auth_admin_password_set = 'true'
  else settings.auth_viewer_password_set = 'true'
  authStatus.value = await api.authStatus()
  ensureAuthDefaults()
  showToast(`${role === 'admin' ? 'Admin' : 'Viewer'}-Passwort gespeichert`)
}

async function loadCredentialIdentities() {
  credentialIdentities.value = await api.credentialIdentities()
}

async function saveCredentialIdentity(form: { id?: string; name: string; username: string; password?: string }) {
  await api.saveCredentialIdentity({ id: form.id || undefined, name: form.name, username: form.username, password: form.password || undefined })
  await loadCredentialIdentities()
  showToast('Identität gespeichert')
}

async function deleteCredentialIdentity(id: string) {
  await api.deleteCredentialIdentity(id)
  await loadCredentialIdentities()
  showToast('Identität entfernt')
}

function addRelay(draft: { id: string; name: string; host: string; sshTarget: string }) {
  const id = sanitizeID(draft.id)
  if (!id) {
    error.value = 'Relay-ID fehlt.'
    return ''
  }
  const ids = relayIds.value.includes(id) ? relayIds.value : [...relayIds.value, id]
  settings['camera.relay.ids'] = ids.join(',')
  settings[relaySettingKey(id, 'name')] = draft.name.trim() || id
  settings[relaySettingKey(id, 'type')] = 'ssh_local_forward'
  settings[relaySettingKey(id, 'host')] = draft.host.trim()
  settings[relaySettingKey(id, 'bind_host')] = '127.0.0.1'
  settings[relaySettingKey(id, 'ssh_target')] = draft.sshTarget.trim()
  settings[relaySettingKey(id, 'auto_start')] = settings[relaySettingKey(id, 'auto_start')] || 'false'
  return id
}

function removeRelay(id: string) {
  settings['camera.relay.ids'] = relayIds.value.filter((relayId) => relayId !== id).join(',')
  for (const field of ['name', 'type', 'host', 'bind_host', 'ssh_target', 'auto_start']) delete settings[relaySettingKey(id, field)]
}

async function relayAction(id: string, action: 'start' | 'stop' | 'restart') {
  error.value = ''
  await api.saveSettings({ ...settings })
  if (action === 'start') await api.startRelay(id)
  if (action === 'stop') await api.stopRelay(id)
  if (action === 'restart') await api.restartRelay(id)
  await refreshStatus()
  showToast(`Relay ${action === 'restart' ? 'neu gestartet' : action === 'start' ? 'gestartet' : 'gestoppt'}`)
}

async function createBackup() {
  backupResult.value = await api.backup()
  showToast('Backup erstellt')
}

async function restoreBackup(path: string) {
  backupResult.value = await api.restore(path)
  showToast('Backup wiederhergestellt')
}

async function createSupportBundle() {
  supportBundleResult.value = await api.supportBundle()
  showToast('Support-Bundle erstellt')
}

export function useSystem() {
  return {
    // state
    settings, status, authStatus, events, credentialIdentities, error, toast, passwordSource,
    viewerPerformanceOptions, backupResult, supportBundleResult,
    // computeds
    relayIds, relayStatuses, cameraBindings, watchdogEnabled, watchdogRestartOnChange, watchdogRestartGo2RTC,
    restartCooldownLabel, versionLabel, versionDetail, viewerPerformanceDescription,
    // actions
    loadAll, refreshStatus, saveSettings, showToast,
    saveCameraPassword, saveAuthPassword,
    loadCredentialIdentities, saveCredentialIdentity, deleteCredentialIdentity,
    addRelay, removeRelay, relayAction, createBackup, restoreBackup, createSupportBundle,
    // helpers
    setBool, boolSetting, formatTime, watchdogDate, levelClass, passwordSourceLabel, sanitizeID,
    relaySettingKey, relayEndpointKey, pathPolicyKey, relayName, relayHost, relayAutoStart, relayStatusFor,
    relayStateLabel, relayStateClass, relayEndpointStateLabel, relayEndpointStateClass, legacyRelayHost, legacyRelayPort,
    normalizedViewerPerformanceMode
  }
}

export type { AuthRole, CredentialIdentity, RelayStatus, SupportBundleResult }
