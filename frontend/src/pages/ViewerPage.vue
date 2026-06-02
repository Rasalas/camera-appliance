<template>
  <header class="topline viewer-topline">
    <div>
      <div class="eyebrow">Kameraansicht · go2rtc</div>
      <h1 class="headline">
        <template v-if="loading">Verbindung <em>wird geprüft.</em></template>
        <template v-else-if="onlineCount">{{ onlineCount }} <em>Stream{{ onlineCount === 1 ? '' : 's' }} bereit.</em></template>
        <template v-else>Kameras <em>nicht bereit.</em></template>
      </h1>
    </div>
    <div class="meta">
      <div>go2rtc · <b>{{ viewer?.go2rtc.online ? 'aktiv' : 'offline' }}</b></div>
      <div>Aliase · <b>{{ viewer?.stream_count ?? 0 }}/{{ slots.length || 5 }}</b></div>
      <div>Stand · <b>{{ checkedAt }}</b></div>
    </div>
  </header>

  <div v-if="error" class="notice err"><span class="tag">FEHLER</span>{{ error }}</div>

  <div class="btn-row viewer-actions">
    <button class="btn primary" :disabled="!!busy" @click="load">{{ busy === 'load' ? 'Lädt…' : 'Neu laden' }}</button>
    <template v-if="canAdmin">
      <button class="btn" :disabled="!!busy" @click="runDiscovery">{{ busy === 'scan' ? 'Suche läuft…' : 'Kameras suchen' }}</button>
      <button class="btn" :disabled="!!busy" @click="render">{{ busy === 'render' ? 'Erzeugt…' : 'go2rtc erzeugen' }}</button>
      <button class="btn ghost" :disabled="!!busy" @click="restartGo2rtc">{{ busy === 'restart' ? 'Startet…' : 'go2rtc neu starten' }}</button>
      <div class="layout-buttons">
        <button class="btn sm" :class="{ live: layoutMode === 'auto' }" :disabled="layoutBusy" @click="setLayoutMode('auto')">Auto</button>
        <button class="btn sm" :class="{ live: layoutMode === 'focus_left' }" :disabled="layoutBusy" @click="setLayoutMode('focus_left')">Fokus links</button>
        <button class="btn sm" :class="{ live: layoutMode === 'focus_right' }" :disabled="layoutBusy" @click="setLayoutMode('focus_right')">Fokus rechts</button>
      </div>
    </template>
    <div class="spacer" />
    <RouterLink v-if="canAdmin" class="btn ghost" to="/einrichtung">Einrichtung</RouterLink>
    <RouterLink v-else-if="auth?.enabled && !auth.authenticated" class="btn ghost" to="/login">Login</RouterLink>
  </div>

  <section class="viewer-grid" :class="layoutClass" :style="layoutGridStyle">
    <article
      v-for="slot in slots"
      :key="slot.alias"
      class="viewer-tile"
      :class="[tileClass(slot), { large: slot.slot.role === 'large' && layoutMode === 'auto' }]"
      :style="layoutTileStyle(slot)"
    >
      <div class="viewer-frame-wrap">
        <div
          v-if="slot.playback?.page_url && isPlayable(slot)"
          class="viewer-frame-transform"
          :class="displayClass(slot)"
          :style="displayStyle(slot)"
        >
          <iframe
            class="viewer-frame"
            :src="slot.playback.page_url"
            :title="slot.label"
            allow="autoplay; fullscreen; picture-in-picture"
            @load="markFrameReady(slot.alias)"
          />
        </div>
        <div v-else class="viewer-placeholder">
          <div class="placeholder-mark">{{ slot.alias }}</div>
          <div>{{ slot.message }}</div>
        </div>
        <div v-if="effectiveState(slot) === 'connecting'" class="viewer-cover">
          <span class="loader-dot" />
          <span>Verbindet</span>
        </div>
      </div>

      <div class="viewer-overlay">
        <div class="slot-tag">
          <span>{{ slot.alias }}</span>
          <span class="pill" :class="stateClass(effectiveState(slot))">{{ stateLabel(effectiveState(slot)) }}</span>
        </div>
        <div>
          <div class="slot-name">{{ slot.label }}</div>
          <div class="slot-meta">
            <span>{{ slot.device?.last_ip || 'keine IP' }}</span>
            <span>{{ slot.binding?.stream_name || slot.slot.default_stream }}</span>
            <span>{{ pathLabel(slot) }}</span>
          </div>
        </div>
      </div>

      <div class="viewer-diagnostics">
        <span
          v-for="diag in slot.diagnostics?.slice(0, 5)"
          :key="`${slot.alias}-${diag.key}`"
          class="diag-chip"
          :class="diag.status"
          :title="diag.message"
        >
          {{ diagLabel(diag.key) }}
        </span>
      </div>
    </article>
    <button
      v-if="focusLayout && canAdmin"
      class="viewer-layout-grabber"
      type="button"
      :style="grabberStyle"
      title="Layout-Breite ziehen"
      @pointerdown="startLayoutDrag"
    />
  </section>

  <section class="panel viewer-summary">
    <div class="panel-head">
      <h2>Diagnose</h2>
      <div class="right">{{ blockingCount }} Slot{{ blockingCount === 1 ? '' : 's' }} mit Aktion</div>
    </div>
    <div v-if="!blockingSlots.length" class="empty">Alle zugeordneten Streams sind für den Viewer vorbereitet.</div>
    <div v-else class="result-list">
      <div v-for="slot in blockingSlots" :key="slot.alias" class="result-row viewer-result" :class="resultClass(slot)">
        <span class="slot">{{ slot.alias }}</span>
        <span class="name">{{ slot.label }}</span>
        <span class="ip">{{ slot.device?.last_ip || 'keine IP' }}</span>
        <span class="stream">{{ stateLabel(slot.state) }}</span>
        <RouterLink v-if="canAdmin" class="action" :to="slot.binding?.device_id ? `/kamera/${slot.binding.device_id}` : '/einrichtung'">
          Öffnen
        </RouterLink>
        <span v-else class="action muted">Viewer</span>
        <span class="message">{{ slot.message }}</span>
      </div>
    </div>
  </section>

  <div class="toast-host">
    <transition name="page"><div v-if="toast" class="toast" :key="toast">{{ toast }}</div></transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '../api/client'
import type { AuthStatus, CameraDisplay, StreamPath, ViewerLayout, ViewerResponse, ViewerSlot, ViewerSlotState } from '../types'

const viewer = ref<ViewerResponse>()
const auth = ref<AuthStatus>()
const loading = ref(true)
const busy = ref<'' | 'load' | 'scan' | 'render' | 'restart'>('')
const layoutBusy = ref(false)
const layoutDraft = ref<ViewerLayout>({ mode: 'auto', focus_slot_id: 'cam5', split_percent: 58, gap_px: 10 })
const error = ref('')
const toast = ref('')
const frameReady = ref<Record<string, boolean>>({})
let refreshTimer = 0
let onAuthChanged: (() => void) | undefined

const slots = computed(() => viewer.value?.slots ?? [])
const onlineCount = computed(() => slots.value.filter((slot) => effectiveState(slot) === 'online').length)
const blockingSlots = computed(() => slots.value.filter((slot) => !['online', 'connecting'].includes(effectiveState(slot))))
const blockingCount = computed(() => blockingSlots.value.length)
const canAdmin = computed(() => auth.value ? (!auth.value.enabled || auth.value.role === 'admin') : false)
const layoutMode = computed(() => layoutDraft.value.mode)
const focusLayout = computed(() => layoutMode.value === 'focus_left' || layoutMode.value === 'focus_right')
const layoutClass = computed(() => ({
  'layout-focus': focusLayout.value,
  'layout-focus-left': layoutMode.value === 'focus_left',
  'layout-focus-right': layoutMode.value === 'focus_right'
}))
const layoutGridStyle = computed(() => {
  if (!focusLayout.value) return {}
  const split = clamp(layoutDraft.value.split_percent || 58, 30, 76)
  const focus = 100 - split
  const gap = clamp(layoutDraft.value.gap_px || 10, 2, 20)
  const columns = layoutMode.value === 'focus_right'
    ? `${split / 2}fr ${split / 2}fr 6px ${focus}fr`
    : `${focus}fr 6px ${split / 2}fr ${split / 2}fr`
  return {
    gridTemplateColumns: columns,
    gridTemplateRows: 'minmax(220px, 1fr) minmax(220px, 1fr)',
    gap: `${gap}px`
  }
})
const grabberStyle = computed(() => ({
  gridColumn: layoutMode.value === 'focus_right' ? '3' : '2',
  gridRow: '1 / span 2'
}))
const checkedAt = computed(() => {
  if (!viewer.value?.checked_at) return '—'
  return new Date(viewer.value.checked_at).toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' })
})

function isPlayable(slot: ViewerSlot) {
  return slot.state === 'online' || slot.state === 'connecting'
}

function effectiveState(slot: ViewerSlot): ViewerSlotState {
  if ((slot.state === 'online' || slot.state === 'connecting') && slot.playback?.page_url && !frameReady.value[slot.alias]) {
    return 'connecting'
  }
  return slot.state
}

function markFrameReady(alias: string) {
  frameReady.value = { ...frameReady.value, [alias]: true }
}

function stateLabel(state: ViewerSlotState) {
  const labels: Record<ViewerSlotState, string> = {
    unassigned: 'leer',
    connecting: 'verbindet',
    online: 'live',
    offline: 'offline',
    credentials_failed: 'login',
    stream_unavailable: 'stream'
  }
  return labels[state]
}

function stateClass(state: ViewerSlotState) {
  if (state === 'online') return 'live'
  if (state === 'connecting' || state === 'stream_unavailable') return 'warn'
  if (state === 'offline' || state === 'credentials_failed') return 'down'
  return ''
}

function tileClass(slot: ViewerSlot) {
  const state = effectiveState(slot)
  return {
    on: state === 'online',
    connecting: state === 'connecting',
    empty: state === 'unassigned',
    off: state === 'offline' || state === 'credentials_failed' || state === 'stream_unavailable'
  }
}

function resultClass(slot: ViewerSlot) {
  return slot.state === 'unassigned' ? '' : 'err'
}

function diagLabel(key: string) {
  const labels: Record<string, string> = {
    assignment: 'Slot',
    network: 'Netz',
    path: 'Pfad',
    credentials: 'Login',
    go2rtc: 'go2rtc'
  }
  return labels[key] || key
}

function pathLabel(slot: ViewerSlot) {
  if (!slot.path) return 'kein Pfad'
  const base = slot.path.kind === 'direct' ? 'direkt' : `Relay ${slot.path.label}`
  const stability = pathStabilityLabel(slot.path)
  return stability ? `${base} · ${stability}` : base
}

function pathStabilityLabel(path: StreamPath) {
  const labels: Record<string, string> = {
    stable: 'stabil',
    warming: 'erholt sich',
    failing: 'instabil',
    unstable: 'wechselbereit',
    failed: 'offline'
  }
  return labels[path.stability] || ''
}

async function load() {
  busy.value = busy.value || 'load'
  error.value = ''
  try {
    viewer.value = await api.viewer()
    syncLayoutDraft()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Viewer konnte nicht geladen werden.'
  } finally {
    loading.value = false
    if (busy.value === 'load') busy.value = ''
  }
}

function syncLayoutDraft() {
  if (!viewer.value?.layout) return
  layoutDraft.value = {
    mode: viewer.value.layout.mode || 'auto',
    focus_slot_id: viewer.value.layout.focus_slot_id || defaultFocusSlotID(),
    split_percent: viewer.value.layout.split_percent || 58,
    gap_px: viewer.value.layout.gap_px || 10
  }
}

function defaultFocusSlotID() {
  return slots.value.find((slot) => slot.slot.role === 'large')?.alias || slots.value[slots.value.length - 1]?.alias || 'cam5'
}

function focusSlotID() {
  const configured = layoutDraft.value.focus_slot_id || defaultFocusSlotID()
  return slots.value.some((slot) => slot.alias === configured) ? configured : defaultFocusSlotID()
}

function layoutTileStyle(slot: ViewerSlot) {
  if (!focusLayout.value) return {}
  const focus = focusSlotID()
  if (slot.alias === focus) {
    return {
      gridColumn: layoutMode.value === 'focus_right' ? '4' : '1',
      gridRow: '1 / span 2'
    }
  }
  const rest = slots.value.filter((candidate) => candidate.alias !== focus)
  const index = Math.max(0, rest.findIndex((candidate) => candidate.alias === slot.alias))
  const col = index % 2
  const row = Math.floor(index / 2) + 1
  if (layoutMode.value === 'focus_right') {
    return { gridColumn: String(col + 1), gridRow: String(row) }
  }
  return { gridColumn: String(col + 3), gridRow: String(row) }
}

function displayClass(slot: ViewerSlot) {
  const display = normalizedDisplay(slot.display)
  return {
    'fit-contain': display.fit_mode === 'contain',
    'rotated-quarter': display.rotation === 90 || display.rotation === 270
  }
}

function displayStyle(slot: ViewerSlot) {
  const display = normalizedDisplay(slot.display)
  const crop = display.crop
  const width = 10000 / crop.width
  const height = 10000 / crop.height
  const left = -(crop.x / crop.width) * 100
  const top = -(crop.y / crop.height) * 100
  const scaleX = display.mirror ? -1 : 1
  const scaleY = display.flip ? -1 : 1
  return {
    left: `${left}%`,
    top: `${top}%`,
    width: `${width}%`,
    height: `${height}%`,
    transform: `rotate(${display.rotation}deg) scaleX(${scaleX}) scaleY(${scaleY})`,
    '--display-fit': display.fit_mode
  }
}

function normalizedDisplay(display?: CameraDisplay): CameraDisplay {
  return {
    rotation: ([0, 90, 180, 270].includes(display?.rotation ?? 0) ? display?.rotation : 0) ?? 0,
    mirror: display?.mirror ?? false,
    flip: display?.flip ?? false,
    fit_mode: display?.fit_mode === 'contain' ? 'contain' : 'cover',
    crop: {
      x: clamp(display?.crop?.x ?? 0, 0, 99),
      y: clamp(display?.crop?.y ?? 0, 0, 99),
      width: clamp(display?.crop?.width ?? 100, 1, 100),
      height: clamp(display?.crop?.height ?? 100, 1, 100)
    }
  }
}

async function setLayoutMode(mode: ViewerLayout['mode']) {
  layoutDraft.value = { ...layoutDraft.value, mode, focus_slot_id: focusSlotID() }
  await saveLayout()
}

async function saveLayout() {
  if (!canAdmin.value) return
  layoutBusy.value = true
  error.value = ''
  try {
    await api.saveSettings({
      'viewer.layout.mode': layoutDraft.value.mode,
      'viewer.layout.focus_slot_id': focusSlotID(),
      'viewer.layout.split_percent': String(clamp(layoutDraft.value.split_percent, 30, 76)),
      'viewer.layout.gap_px': String(clamp(layoutDraft.value.gap_px, 2, 20))
    })
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Layout konnte nicht gespeichert werden.'
  } finally {
    layoutBusy.value = false
  }
}

function startLayoutDrag(event: PointerEvent) {
  if (!focusLayout.value || !canAdmin.value) return
  const grid = (event.currentTarget as HTMLElement).closest('.viewer-grid')
  if (!(grid instanceof HTMLElement)) return
  event.preventDefault()
  const rect = grid.getBoundingClientRect()
  const pointerId = event.pointerId
  ;(event.currentTarget as HTMLElement).setPointerCapture(pointerId)
  const move = (moveEvent: PointerEvent) => {
    const relative = clamp(Math.round(((moveEvent.clientX - rect.left) / rect.width) * 100), 20, 80)
    layoutDraft.value.split_percent = layoutMode.value === 'focus_right' ? clamp(relative, 30, 76) : clamp(100 - relative, 30, 76)
  }
  const up = async () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    await saveLayout()
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
}

function clamp(value: number, min: number, max: number) {
  if (!Number.isFinite(value)) return min
  return Math.min(max, Math.max(min, value))
}

async function refreshAuth() {
  try {
    auth.value = await api.authStatus()
  } catch {
    auth.value = undefined
  }
}

async function runDiscovery() {
  busy.value = 'scan'
  error.value = ''
  try {
    const result = await api.discover()
    toast.value = `${result.devices.length} Gerät(e) gefunden`
    setTimeout(() => (toast.value = ''), 2400)
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Kamerasuche fehlgeschlagen.'
  } finally {
    busy.value = ''
  }
}

async function render() {
  busy.value = 'render'
  error.value = ''
  try {
    const result = await api.renderGo2RTC()
    frameReady.value = {}
    toast.value = `${result.rendered_streams} Stream(s) erzeugt`
    setTimeout(() => (toast.value = ''), 2400)
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'go2rtc konnte nicht erzeugt werden.'
  } finally {
    busy.value = ''
  }
}

async function restartGo2rtc() {
  busy.value = 'restart'
  error.value = ''
  try {
    await api.restartGo2RTC()
    frameReady.value = {}
    toast.value = 'go2rtc neu gestartet'
    setTimeout(() => (toast.value = ''), 2400)
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'go2rtc konnte nicht neu gestartet werden.'
  } finally {
    busy.value = ''
  }
}

onMounted(() => {
  void refreshAuth()
  onAuthChanged = () => {
    void refreshAuth()
  }
  window.addEventListener('auth-changed', onAuthChanged)
  void load()
  refreshTimer = window.setInterval(() => {
    if (!busy.value) void load()
  }, 15000)
})

onBeforeUnmount(() => {
  window.clearInterval(refreshTimer)
  if (onAuthChanged) window.removeEventListener('auth-changed', onAuthChanged)
})
</script>
