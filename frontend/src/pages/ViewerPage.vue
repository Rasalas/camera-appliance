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
      <div>Modus · <b>{{ performanceName }}</b></div>
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
    </template>
    <div class="layout-buttons viewer-layout-controls">
      <select class="layout-select" :value="activeLayoutID" :disabled="layoutBusy" @change="setLayoutFromEvent">
        <option v-for="option in layoutOptions" :key="option.id" :value="option.id">{{ option.name }}</option>
      </select>
      <select class="layout-select performance-select" :value="performanceMode" :disabled="performanceBusy" @change="setPerformanceFromEvent">
        <option v-for="option in performanceOptions" :key="option.id" :value="option.id">{{ option.name }}</option>
      </select>
      <template v-if="splitLayout">
        <button class="btn sm" :class="{ live: focusSide === 'left' }" :disabled="layoutBusy" @click="setFocusSide('focus_left')">Links</button>
        <button class="btn sm" :class="{ live: focusSide === 'middle' }" :disabled="layoutBusy" @click="setFocusSide('focus_middle')">Mitte</button>
        <button class="btn sm" :class="{ live: focusSide === 'right' }" :disabled="layoutBusy" @click="setFocusSide('focus_right')">Rechts</button>
      </template>
    </div>
    <div class="spacer" />
    <RouterLink v-if="canAdmin" class="btn ghost" to="/einrichtung">Einrichtung</RouterLink>
    <RouterLink v-else-if="auth?.enabled && !auth.authenticated" class="btn ghost" to="/login">Login</RouterLink>
  </div>

  <section class="viewer-grid" :class="layoutClass" :style="layoutGridStyle">
    <article
      v-for="slot in visibleSlots"
      :key="slot.alias"
      class="viewer-tile"
      :class="[tileClass(slot), { large: isFocusTile(slot), portrait: isPortraitTile(slot), reorderable: canReorderLayout, dragging: draggedSlotAlias === slot.alias, 'drop-target': dropSlotAlias === slot.alias && draggedSlotAlias !== slot.alias }]"
      :style="layoutTileStyle(slot)"
      :data-slot-alias="slot.alias"
      :title="canReorderLayout ? 'Kamera ziehen' : undefined"
      @pointerdown="startTilePointerDrag($event, slot)"
    >
      <div class="viewer-frame-wrap">
        <div
          v-if="shouldRenderPlayer(slot)"
          class="viewer-frame-transform"
          :class="displayClass(slot)"
          :style="displayStyle(slot)"
        >
          <iframe
            class="viewer-frame"
            :src="slot.playback?.page_url || ''"
            :title="slot.label"
            :loading="iframeLoading(slot)"
            allow="autoplay; fullscreen; picture-in-picture"
            @load="markFrameReady(slot.alias)"
          />
        </div>
        <div v-else class="viewer-placeholder" :class="{ paused: isPausedByPerformance(slot) }">
          <div class="placeholder-mark">{{ slot.alias }}</div>
          <div>{{ placeholderMessage(slot) }}</div>
        </div>
        <div v-if="isPausedByPerformance(slot)" class="viewer-cover performance-cover">
          <span>Standby</span>
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
            <span v-if="diagnosticMode">{{ streamStatusLabel(slot) }}</span>
          </div>
        </div>
      </div>

      <div class="viewer-diagnostics">
        <span
          v-for="diag in visibleDiagnostics(slot)"
          :key="`${slot.alias}-${diag.key}`"
          class="diag-chip"
          :class="diag.status"
          :title="diag.message"
        >
          {{ diagLabel(diag.key) }}
        </span>
      </div>
    </article>
    <div v-if="activeLayoutID === 'custom' && draggedSlotAlias" class="viewer-drop-zones" aria-hidden="true">
      <button
        v-for="zone in customDropZones"
        :key="zone.id"
        class="viewer-drop-zone"
        :class="[`zone-${zone.id}`, { active: dropZoneID === zone.id }]"
        type="button"
        :data-layout-zone="zone.id"
      >
        {{ zone.label }}
      </button>
    </div>
    <template v-if="canAdmin">
      <button
        v-for="grabber in grabberStyles"
        :key="grabber.handle"
        class="viewer-layout-grabber"
        type="button"
        :style="grabber.style"
        title="Layout-Breite ziehen"
        @pointerdown="startLayoutDrag($event, grabber.handle)"
      />
      <button
        v-for="grabber in customGrabberStyles"
        :key="grabber.handle"
        class="viewer-layout-grabber floating"
        :class="grabber.kind === 'column' ? 'custom-col' : 'custom-row'"
        type="button"
        :style="grabber.style"
        :title="grabber.kind === 'column' ? 'Spaltenbreite ziehen' : 'Zeilenhöhe ziehen'"
        @pointerdown="startCustomLayoutDrag($event, grabber.kind, grabber.index)"
      />
    </template>
  </section>

  <section v-if="diagnosticMode || performanceMode !== 'quality'" class="panel viewer-performance">
    <div class="panel-head">
      <h2>Performance</h2>
      <div class="right">{{ activePlayerCount }}/{{ visibleSlots.length }} Player aktiv · {{ totalConsumers }} Consumer</div>
    </div>
    <div class="performance-grid">
      <div v-for="slot in visibleSlots" :key="`perf-${slot.alias}`" class="performance-row">
        <span class="slot">{{ slot.alias }}</span>
        <span class="name">{{ isPausedByPerformance(slot) ? 'pausiert' : playerStateLabel(slot) }}</span>
        <span class="stream">P {{ slot.stream?.producers ?? 0 }} · C {{ slot.stream?.consumers ?? 0 }}</span>
        <span class="message">{{ streamStatusLabel(slot) }}</span>
      </div>
    </div>
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
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import type {
  AuthStatus,
  CameraDisplay,
  StreamPath,
  ViewerCustomLayout,
  ViewerCustomLayoutCell,
  ViewerLayoutID,
  ViewerLayoutMode,
  ViewerLayoutOption,
  ViewerPerformanceMode,
  ViewerPerformanceOption,
  ViewerResponse,
  ViewerSlot,
  ViewerSlotState
} from '../types'

type LayoutDraft = {
  id: ViewerLayoutID
  mode: ViewerLayoutMode
  focus_slot_id: string
  slot_order: string[]
  split_percent: number
  gap_px: number
  custom: ViewerCustomLayout
}

type DropZoneID = 'left' | 'middle' | 'right' | 'top' | 'bottom'
type CustomGrabberKind = 'column' | 'row'

const fallbackPerformanceOptions: ViewerPerformanceOption[] = [
  { id: 'quality', name: 'Qualität', description: 'Alle sichtbaren Streams sofort live laden.' },
  { id: 'balanced', name: 'Balanciert', description: 'Nebenansichten lazy laden und primäre Ansicht priorisieren.' },
  { id: 'low', name: 'Niedrig', description: 'Nur die primäre Ansicht live laden, Nebenansichten pausieren.' },
  { id: 'diagnostic', name: 'Diagnose', description: 'Alle Streams live laden und Producer/Consumer sichtbar machen.' }
]
const defaultCustomColumns = [29, 29, 6, 36]
const defaultCustomRows = [50, 50]
const fallbackLayoutOptions: ViewerLayoutOption[] = [
  { id: 'grid_2x2', name: '2x2', description: 'Vier gleich große Kameras im Raster.' },
  { id: 'four_plus_large', name: '4 plus groß', description: 'Vier Raster-Kameras mit einer prominenten Ansicht.' },
  { id: 'vertical_plus_grid', name: 'Vertikal plus Raster', description: 'Eine hochformatige Kamera neben einem Raster.' },
  { id: 'large_only', name: 'Große Ansicht', description: 'Nur die prominente Kamera bildschirmfüllend.' },
  { id: 'custom', name: 'Frei', description: 'Kameras per Drag-and-drop auf Zonen und Größen legen.' }
]
const customDropZones: Array<{ id: DropZoneID; label: string }> = [
  { id: 'left', label: 'Links' },
  { id: 'middle', label: 'Mitte' },
  { id: 'right', label: 'Rechts' },
  { id: 'top', label: 'Oben' },
  { id: 'bottom', label: 'Unten' }
]

const route = useRoute()
const router = useRouter()
const viewer = ref<ViewerResponse>()
const auth = ref<AuthStatus>()
const loading = ref(true)
const busy = ref<'' | 'load' | 'scan' | 'render' | 'restart'>('')
const layoutBusy = ref(false)
const performanceBusy = ref(false)
const performanceMode = ref<ViewerPerformanceMode>('quality')
const layoutDraft = ref<LayoutDraft>({
  id: 'four_plus_large',
  mode: 'auto',
  focus_slot_id: 'cam5',
  slot_order: [],
  split_percent: 58,
  gap_px: 10,
  custom: { columns: defaultCustomColumns, rows: defaultCustomRows, cells: [] }
})
const error = ref('')
const toast = ref('')
const frameReady = ref<Record<string, boolean>>({})
const draggedSlotAlias = ref('')
const dropSlotAlias = ref('')
const dropZoneID = ref<DropZoneID | ''>('')
let refreshTimer = 0
let onAuthChanged: (() => void) | undefined
let stopPointerTileDrag: (() => void) | undefined

const slots = computed(() => viewer.value?.slots ?? [])
const orderedSlots = computed(() => orderSlotsByAlias(slots.value, layoutDraft.value.slot_order))
const onlineCount = computed(() => slots.value.filter((slot) => effectiveState(slot) === 'online').length)
const blockingSlots = computed(() => slots.value.filter((slot) => !['online', 'connecting'].includes(effectiveState(slot))))
const blockingCount = computed(() => blockingSlots.value.length)
const canAdmin = computed(() => auth.value ? (!auth.value.enabled || auth.value.role === 'admin') : false)
const canReorderLayout = computed(() => canAdmin.value && !layoutBusy.value && visibleSlots.value.length > 1)
const layoutOptions = computed(() => viewer.value?.layout.options?.length ? viewer.value.layout.options : fallbackLayoutOptions)
const performanceOptions = computed(() => viewer.value?.performance.options?.length ? viewer.value.performance.options : fallbackPerformanceOptions)
const performanceName = computed(() => performanceOptions.value.find((option) => option.id === performanceMode.value)?.name || 'Qualität')
const diagnosticMode = computed(() => performanceMode.value === 'diagnostic')
const activeLayoutID = computed(() => normalizedLayoutID(layoutDraft.value.id))
const layoutMode = computed(() => layoutDraft.value.mode)
const splitLayout = computed(() => activeLayoutID.value === 'four_plus_large' || activeLayoutID.value === 'vertical_plus_grid')
const focusSide = computed<'left' | 'middle' | 'right'>(() => {
  if (layoutMode.value === 'focus_left') return 'left'
  if (layoutMode.value === 'focus_middle') return 'middle'
  return 'right'
})
const visibleSlots = computed(() => {
  const focus = focusSlotID()
  if (activeLayoutID.value === 'large_only') {
    return orderedSlots.value.filter((slot) => slot.alias === focus).slice(0, 1)
  }
  if (activeLayoutID.value === 'custom') {
    const byAlias = new Map(orderedSlots.value.map((slot) => [slot.alias, slot]))
    const seen = new Set<string>()
    const placed: ViewerSlot[] = []
    for (const cell of normalizedCustomLayout(layoutDraft.value.custom).cells) {
      const slot = byAlias.get(cell.slot_id)
      if (!slot || seen.has(slot.alias)) continue
      placed.push(slot)
      seen.add(slot.alias)
    }
    return [...placed, ...orderedSlots.value.filter((slot) => !seen.has(slot.alias))]
  }
  if (activeLayoutID.value === 'grid_2x2') {
    return preferredGridSlots(focus).slice(0, 4)
  }
  const focusSlot = orderedSlots.value.find((slot) => slot.alias === focus)
  const grid = preferredGridSlots(focus).slice(0, 4)
  return focusSlot ? [focusSlot, ...grid] : grid
})
const layoutClass = computed(() => ({
  [`layout-${activeLayoutID.value}`]: true,
  'layout-focus': splitLayout.value,
  'layout-focus-left': splitLayout.value && focusSide.value === 'left',
  'layout-focus-middle': splitLayout.value && focusSide.value === 'middle',
  'layout-focus-right': splitLayout.value && focusSide.value === 'right'
}))
const layoutGridStyle = computed(() => {
  if (activeLayoutID.value === 'custom') {
    const custom = normalizedCustomLayout(layoutDraft.value.custom)
    return {
      gridTemplateColumns: custom.columns.map((column) => `${column}fr`).join(' '),
      gridTemplateRows: custom.rows.map((row) => `minmax(180px, ${row}fr)`).join(' '),
      gap: `${clamp(layoutDraft.value.gap_px || 10, 2, 20)}px`
    }
  }
  if (!splitLayout.value) return {}
  const split = clamp(layoutDraft.value.split_percent || 58, 30, 76)
  const focus = 100 - split
  const gap = clamp(layoutDraft.value.gap_px || 10, 2, 20)
  let columns = focusSide.value === 'right'
    ? `${split / 2}fr ${split / 2}fr 6px ${focus}fr`
    : `${focus}fr 6px ${split / 2}fr ${split / 2}fr`
  if (focusSide.value === 'middle') {
    columns = `${split / 2}fr 6px ${focus}fr 6px ${split / 2}fr`
  }
  return {
    gridTemplateColumns: columns,
    gridTemplateRows: 'minmax(220px, 1fr) minmax(220px, 1fr)',
    gap: `${gap}px`
  }
})
const grabberStyles = computed(() => {
  if (!splitLayout.value) return []
  if (focusSide.value === 'middle') {
    return [
      { handle: 'middle_left' as const, style: { gridColumn: '2', gridRow: '1 / span 2' } },
      { handle: 'middle_right' as const, style: { gridColumn: '4', gridRow: '1 / span 2' } }
    ]
  }
  return [{
    handle: 'single' as const,
    style: {
      gridColumn: focusSide.value === 'right' ? '3' : '2',
      gridRow: '1 / span 2'
    }
  }]
})
const customGrabberStyles = computed(() => {
  if (activeLayoutID.value !== 'custom') return []
  const custom = normalizedCustomLayout(layoutDraft.value.custom)
  const columnTotal = sum(custom.columns)
  const rowTotal = sum(custom.rows)
  let left = 0
  let top = 0
  const grabbers: Array<{ handle: string; kind: CustomGrabberKind; index: number; style: Record<string, string> }> = []
  for (let index = 0; index < custom.columns.length - 1; index += 1) {
    left += custom.columns[index]
    grabbers.push({
      handle: `column-${index}`,
      kind: 'column',
      index,
      style: { left: `${(left / columnTotal) * 100}%` }
    })
  }
  for (let index = 0; index < custom.rows.length - 1; index += 1) {
    top += custom.rows[index]
    grabbers.push({
      handle: `row-${index}`,
      kind: 'row',
      index,
      style: { top: `${(top / rowTotal) * 100}%` }
    })
  }
  return grabbers
})
const primaryLiveAlias = computed(() => {
  if (!visibleSlots.value.length) return ''
  if (activeLayoutID.value === 'custom') {
    return prominentCustomSlot(normalizedCustomLayout(layoutDraft.value.custom)) || visibleSlots.value[0].alias
  }
  if (splitLayout.value || activeLayoutID.value === 'large_only') return focusSlotID()
  return visibleSlots.value[0]?.alias || ''
})
const activePlayerCount = computed(() => visibleSlots.value.filter((slot) => shouldRenderPlayer(slot)).length)
const totalConsumers = computed(() => slots.value.reduce((total, slot) => total + (slot.stream?.consumers ?? 0), 0))
const checkedAt = computed(() => {
  if (!viewer.value?.checked_at) return '—'
  return new Date(viewer.value.checked_at).toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' })
})

function isPlayable(slot: ViewerSlot) {
  return slot.state === 'online' || slot.state === 'connecting'
}

function shouldRenderPlayer(slot: ViewerSlot) {
  if (!slot.playback?.page_url || !isPlayable(slot)) return false
  if (performanceMode.value === 'low') return slot.alias === primaryLiveAlias.value
  return true
}

function isPausedByPerformance(slot: ViewerSlot) {
  return performanceMode.value === 'low' && !!slot.playback?.page_url && isPlayable(slot) && slot.alias !== primaryLiveAlias.value
}

function iframeLoading(slot: ViewerSlot): 'eager' | 'lazy' {
  if (performanceMode.value === 'balanced' && slot.alias !== primaryLiveAlias.value) return 'lazy'
  return 'eager'
}

function placeholderMessage(slot: ViewerSlot) {
  if (isPausedByPerformance(slot)) return 'Im Low-Modus pausiert.'
  return slot.message
}

function effectiveState(slot: ViewerSlot): ViewerSlotState {
  if ((slot.state === 'online' || slot.state === 'connecting') && shouldRenderPlayer(slot) && !frameReady.value[slot.alias]) {
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
    paused: isPausedByPerformance(slot),
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
    go2rtc: 'go2rtc',
    stream: 'Stream'
  }
  return labels[key] || key
}

function visibleDiagnostics(slot: ViewerSlot) {
  const diagnostics = slot.diagnostics ?? []
  return diagnosticMode.value ? diagnostics : diagnostics.slice(0, 5)
}

function streamStatusLabel(slot: ViewerSlot) {
  const stream = slot.stream
  if (!stream?.configured) return 'nicht konfiguriert'
  return `${stream.producers ?? 0} Producer · ${stream.consumers ?? 0} Consumer`
}

function playerStateLabel(slot: ViewerSlot) {
  if (!shouldRenderPlayer(slot)) return stateLabel(effectiveState(slot))
  if (frameReady.value[slot.alias]) return 'Player geladen'
  return 'Player lädt'
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
    syncPerformanceDraft()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Viewer konnte nicht geladen werden.'
  } finally {
    loading.value = false
    if (busy.value === 'load') busy.value = ''
  }
}

function syncPerformanceDraft() {
  const routeMode = routePerformanceMode()
  performanceMode.value = routeMode || normalizedPerformanceMode(viewer.value?.performance?.mode)
}

function syncLayoutDraft() {
  if (!viewer.value?.layout) return
  const serverLayout = viewer.value.layout
  const routeID = routeLayoutID()
  const id = routeID || normalizedLayoutID(serverLayout.id || layoutIDFromMode(serverLayout.mode))
  const routeMode = routeFocusMode()
  const mode = routeMode || normalizedLayoutMode(serverLayout.mode, id)
  layoutDraft.value = {
    id,
    mode,
    focus_slot_id: serverLayout.focus_slot_id || defaultFocusSlotID(),
    slot_order: serverLayout.slot_order?.length ? serverLayout.slot_order : slots.value.map((slot) => slot.alias),
    split_percent: serverLayout.split_percent || defaultSplitPercent(id),
    gap_px: serverLayout.gap_px || 8,
    custom: normalizedCustomLayout(serverLayout.custom?.cells?.length ? serverLayout.custom : defaultCustomLayoutForSlots(serverLayout.focus_slot_id || defaultFocusSlotID()))
  }
}

function defaultFocusSlotID() {
  return slots.value.find((slot) => slot.slot.role === 'large')?.alias || slots.value[slots.value.length - 1]?.alias || 'cam5'
}

function focusSlotID() {
  const configured = layoutDraft.value.focus_slot_id || defaultFocusSlotID()
  return slots.value.some((slot) => slot.alias === configured) ? configured : defaultFocusSlotID()
}

function preferredGridSlots(excludeAlias: string) {
  const regular = orderedSlots.value.filter((slot) => slot.alias !== excludeAlias && slot.slot.role !== 'large')
  const fallback = orderedSlots.value.filter((slot) => slot.alias !== excludeAlias && slot.slot.role === 'large')
  return [...regular, ...fallback]
}

function normalizedCustomLayout(layout?: ViewerCustomLayout): ViewerCustomLayout {
  const fallback = defaultCustomLayoutForSlots()
  const source = layout?.cells?.length ? layout : fallback
  const columns = normalizedCustomWeights(source.columns, defaultCustomColumns, 1, 6)
  const rows = normalizedCustomWeights(source.rows, defaultCustomRows, 1, 4)
  const slotAliases = new Set(slots.value.map((slot) => slot.alias))
  const seen = new Set<string>()
  const occupied = new Set<string>()
  const cells: ViewerCustomLayoutCell[] = []

  for (const cell of source.cells || []) {
    if (!slotAliases.has(cell.slot_id) || seen.has(cell.slot_id)) continue
    const normalized = normalizedCustomCell(cell, columns.length, rows.length)
    const coordinates = customCellCoordinates(normalized)
    if (coordinates.some((coordinate) => occupied.has(coordinate))) continue
    for (const coordinate of coordinates) occupied.add(coordinate)
    seen.add(normalized.slot_id)
    cells.push(normalized)
  }

  return {
    columns,
    rows,
    cells: fillMissingCustomCells(cells, columns.length, rows.length)
  }
}

function normalizedCustomWeights(values: number[] | undefined, fallback: number[], minLength: number, maxLength: number) {
  const source = Array.isArray(values) && values.length >= minLength && values.length <= maxLength ? values : fallback
  const weights = source.map((value) => clamp(Math.round(Number(value) || 0), 1, 100))
  const total = sum(weights)
  if (total > 0 && total < 20) {
    return weights.map((value) => clamp(Math.round((value / total) * 100), 1, 100))
  }
  return weights
}

function normalizedCustomCell(cell: ViewerCustomLayoutCell, columnCount: number, rowCount: number): ViewerCustomLayoutCell {
  const column = clamp(Math.round(cell.column || 1), 1, columnCount)
  const row = clamp(Math.round(cell.row || 1), 1, rowCount)
  return {
    slot_id: cell.slot_id,
    column,
    row,
    column_span: clamp(Math.round(cell.column_span || 1), 1, columnCount - column + 1),
    row_span: clamp(Math.round(cell.row_span || 1), 1, rowCount - row + 1)
  }
}

function defaultCustomLayoutForSlots(focusAlias = focusSlotID(), side: 'left' | 'middle' | 'right' = 'right') {
  const zone = side === 'left' ? 'left' : side === 'middle' ? 'middle' : 'right'
  const leadCell = customLeadCell(focusAlias, zone, defaultCustomColumns.length, defaultCustomRows.length)
  return customLayoutFromLead(focusAlias, leadCell, defaultCustomColumns, defaultCustomRows)
}

function customLeadCell(slotAlias: string, zone: DropZoneID, columnCount: number, rowCount: number): ViewerCustomLayoutCell {
  if (zone === 'left') {
    return { slot_id: slotAlias, column: 1, row: 1, column_span: 1, row_span: rowCount }
  }
  if (zone === 'middle') {
    const span = columnCount >= 4 ? 2 : 1
    const column = columnCount >= 4 ? 2 : Math.max(1, Math.ceil(columnCount / 2))
    return { slot_id: slotAlias, column, row: 1, column_span: span, row_span: rowCount }
  }
  if (zone === 'top') {
    return { slot_id: slotAlias, column: 1, row: 1, column_span: columnCount, row_span: 1 }
  }
  if (zone === 'bottom') {
    return { slot_id: slotAlias, column: 1, row: rowCount, column_span: columnCount, row_span: 1 }
  }
  return { slot_id: slotAlias, column: columnCount, row: 1, column_span: 1, row_span: rowCount }
}

function buildCustomLayoutWithLead(sourceAlias: string, zone: DropZoneID) {
  const current = normalizedCustomLayout(layoutDraft.value.custom)
  const columns = current.columns.length === defaultCustomColumns.length ? current.columns : defaultCustomColumns
  const rows = current.rows.length === defaultCustomRows.length ? current.rows : defaultCustomRows
  const leadCell = customLeadCell(sourceAlias, zone, columns.length, rows.length)
  return customLayoutFromLead(sourceAlias, leadCell, columns, rows)
}

function customLayoutFromLead(sourceAlias: string, leadCell: ViewerCustomLayoutCell, columns: number[], rows: number[]): ViewerCustomLayout {
  const occupied = new Set(customCellCoordinates(leadCell))
  const positions = freeCustomPositions(occupied, columns.length, rows.length)
  const fallbackPosition = { column: 1, row: 1, column_span: 1, row_span: 1 }
  const cells: ViewerCustomLayoutCell[] = [leadCell]
  let index = 0
  for (const alias of normalizedSlotOrder()) {
    if (alias === sourceAlias) continue
    const position = positions[index % Math.max(positions.length, 1)] || fallbackPosition
    cells.push({ slot_id: alias, ...position })
    index += 1
  }
  return { columns: [...columns], rows: [...rows], cells }
}

function fillMissingCustomCells(cells: ViewerCustomLayoutCell[], columnCount: number, rowCount: number) {
  const seen = new Set(cells.map((cell) => cell.slot_id))
  const occupied = new Set(cells.flatMap((cell) => customCellCoordinates(cell)))
  const positions = freeCustomPositions(occupied, columnCount, rowCount)
  const fallbackPosition = { column: 1, row: 1, column_span: 1, row_span: 1 }
  const completed = [...cells]
  let index = 0
  for (const slot of orderedSlots.value) {
    if (seen.has(slot.alias)) continue
    const position = positions[index % Math.max(positions.length, 1)] || fallbackPosition
    completed.push({ slot_id: slot.alias, ...position })
    index += 1
  }
  return completed
}

function freeCustomPositions(occupied: Set<string>, columnCount: number, rowCount: number) {
  const positions: Array<Omit<ViewerCustomLayoutCell, 'slot_id'>> = []
  for (let row = 1; row <= rowCount; row += 1) {
    for (let column = 1; column <= columnCount; column += 1) {
      if (!occupied.has(`${column}:${row}`)) {
        positions.push({ column, row, column_span: 1, row_span: 1 })
      }
    }
  }
  return positions
}

function customCellCoordinates(cell: ViewerCustomLayoutCell) {
  const coordinates: string[] = []
  for (let row = cell.row; row < cell.row + cell.row_span; row += 1) {
    for (let column = cell.column; column < cell.column + cell.column_span; column += 1) {
      coordinates.push(`${column}:${row}`)
    }
  }
  return coordinates
}

function customCellForAlias(alias: string) {
  return normalizedCustomLayout(layoutDraft.value.custom).cells.find((cell) => cell.slot_id === alias)
}

function swapCustomCells(sourceAlias: string, targetAlias: string): ViewerCustomLayout {
  const custom = normalizedCustomLayout(layoutDraft.value.custom)
  const source = custom.cells.find((cell) => cell.slot_id === sourceAlias)
  const target = custom.cells.find((cell) => cell.slot_id === targetAlias)
  if (!source || !target) return custom
  const sourceGeometry = customCellGeometry(source)
  const targetGeometry = customCellGeometry(target)
  return {
    ...custom,
    cells: custom.cells.map((cell) => {
      if (cell.slot_id === sourceAlias) return { slot_id: sourceAlias, ...targetGeometry }
      if (cell.slot_id === targetAlias) return { slot_id: targetAlias, ...sourceGeometry }
      return cell
    })
  }
}

function customCellGeometry(cell: ViewerCustomLayoutCell) {
  return {
    column: cell.column,
    row: cell.row,
    column_span: cell.column_span,
    row_span: cell.row_span
  }
}

function prominentCustomSlot(custom: ViewerCustomLayout) {
  let alias = ''
  let area = 0
  for (const cell of custom.cells) {
    const cellArea = cell.column_span * cell.row_span
    if (cellArea > area) {
      alias = cell.slot_id
      area = cellArea
    }
  }
  return alias
}

function orderSlotsByAlias(items: ViewerSlot[], order: string[]) {
  const byAlias = new Map(items.map((slot) => [slot.alias, slot]))
  const seen = new Set<string>()
  const ordered: ViewerSlot[] = []
  for (const alias of order) {
    const slot = byAlias.get(alias)
    if (!slot || seen.has(alias)) continue
    ordered.push(slot)
    seen.add(alias)
  }
  for (const slot of items) {
    if (!seen.has(slot.alias)) ordered.push(slot)
  }
  return ordered
}

function normalizedSlotOrder() {
  return orderSlotsByAlias(slots.value, layoutDraft.value.slot_order).map((slot) => slot.alias)
}

function swappedSlotOrder(sourceAlias: string, targetAlias: string) {
  const order = normalizedSlotOrder()
  const sourceIndex = order.indexOf(sourceAlias)
  const targetIndex = order.indexOf(targetAlias)
  if (sourceIndex === -1 || targetIndex === -1) return order
  ;[order[sourceIndex], order[targetIndex]] = [order[targetIndex], order[sourceIndex]]
  return order
}

function gridSideForAlias(alias: string, focusAlias: string): 'left' | 'right' {
  if (focusSide.value === 'left') return 'right'
  if (focusSide.value === 'right') return 'left'
  const index = preferredGridSlots(focusAlias).findIndex((slot) => slot.alias === alias)
  return index >= 2 ? 'right' : 'left'
}

function focusModeForSide(side: 'left' | 'middle' | 'right'): ViewerLayoutMode {
  if (side === 'left') return 'focus_left'
  if (side === 'middle') return 'focus_middle'
  return 'focus_right'
}

function tileAliasFromPoint(x: number, y: number) {
  const element = document.elementFromPoint(x, y)
  if (!(element instanceof HTMLElement)) return ''
  return element.closest<HTMLElement>('.viewer-tile')?.dataset.slotAlias || ''
}

function layoutZoneFromPoint(x: number, y: number): DropZoneID | '' {
  const element = document.elementFromPoint(x, y)
  if (!(element instanceof HTMLElement)) return ''
  const zone = element.closest<HTMLElement>('[data-layout-zone]')?.dataset.layoutZone
  return zone === 'left' || zone === 'middle' || zone === 'right' || zone === 'top' || zone === 'bottom' ? zone : ''
}

function layoutTileStyle(slot: ViewerSlot) {
  if (activeLayoutID.value === 'custom') {
    const cell = customCellForAlias(slot.alias)
    if (!cell) return {}
    return {
      gridColumn: `${cell.column} / span ${cell.column_span}`,
      gridRow: `${cell.row} / span ${cell.row_span}`
    }
  }
  if (!splitLayout.value) return {}
  const focus = focusSlotID()
  if (slot.alias === focus) {
    if (focusSide.value === 'middle') {
      return { gridColumn: '3', gridRow: '1 / span 2' }
    }
    return {
      gridColumn: focusSide.value === 'right' ? '4' : '1',
      gridRow: '1 / span 2'
    }
  }
  const rest = preferredGridSlots(focus)
  const index = Math.max(0, rest.findIndex((candidate) => candidate.alias === slot.alias))
  if (focusSide.value === 'middle') {
    return {
      gridColumn: index < 2 ? '1' : '5',
      gridRow: String((index % 2) + 1)
    }
  }
  const col = index % 2
  const row = Math.floor(index / 2) + 1
  if (focusSide.value === 'right') {
    return { gridColumn: String(col + 1), gridRow: String(row) }
  }
  return { gridColumn: String(col + 3), gridRow: String(row) }
}

function isFocusTile(slot: ViewerSlot) {
  if (activeLayoutID.value === 'custom') {
    const cell = customCellForAlias(slot.alias)
    return !!cell && (cell.column_span > 1 || cell.row_span > 1)
  }
  return splitLayout.value && slot.alias === focusSlotID()
}

function isPortraitTile(slot: ViewerSlot) {
  return activeLayoutID.value === 'vertical_plus_grid' && slot.alias === focusSlotID()
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
  const rotated = display.rotation === 90 || display.rotation === 270
  return {
    left: rotated ? '50%' : `${left}%`,
    top: rotated ? '50%' : `${top}%`,
    width: rotated ? `calc(${width / 100} * 100cqh)` : `${width}%`,
    height: rotated ? `calc(${height / 100} * 100cqw)` : `${height}%`,
    transform: `${rotated ? 'translate(-50%, -50%) ' : ''}rotate(${display.rotation}deg) scaleX(${scaleX}) scaleY(${scaleY})`,
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

function normalizedLayoutID(raw?: string): ViewerLayoutID {
  if (raw === 'grid_2x2' || raw === 'four_plus_large' || raw === 'vertical_plus_grid' || raw === 'large_only' || raw === 'custom') return raw
  return 'four_plus_large'
}

function layoutIDFromMode(raw?: string): ViewerLayoutID {
  if (raw === 'grid_2x2' || raw === 'vertical_plus_grid' || raw === 'large_only' || raw === 'custom') return raw
  return 'four_plus_large'
}

function normalizedLayoutMode(raw: string | undefined, id: ViewerLayoutID): ViewerLayoutMode {
  if (id === 'four_plus_large' || id === 'vertical_plus_grid') {
    if (raw === 'focus_left' || raw === 'focus_middle' || raw === 'focus_right') return raw
    return id === 'vertical_plus_grid' ? 'focus_right' : 'auto'
  }
  if (id === 'custom') return 'custom'
  return id
}

function defaultSplitPercent(id: ViewerLayoutID) {
  return id === 'vertical_plus_grid' ? 64 : 58
}

function normalizedPerformanceMode(raw?: string): ViewerPerformanceMode {
  if (raw === 'balanced' || raw === 'low' || raw === 'diagnostic') return raw
  return 'quality'
}

function routeLayoutID(): ViewerLayoutID | undefined {
  const raw = Array.isArray(route.query.layout) ? route.query.layout[0] : route.query.layout
  if (!raw) return undefined
  return normalizedLayoutID(raw)
}

function routeFocusMode(): ViewerLayoutMode | undefined {
  const raw = Array.isArray(route.query.side) ? route.query.side[0] : route.query.side
  if (raw === 'left') return 'focus_left'
  if (raw === 'middle') return 'focus_middle'
  if (raw === 'right') return 'focus_right'
  return undefined
}

function routePerformanceMode(): ViewerPerformanceMode | undefined {
  const raw = Array.isArray(route.query.perf) ? route.query.perf[0] : route.query.perf
  if (!raw) return undefined
  return normalizedPerformanceMode(raw)
}

async function setLayoutFromEvent(event: Event) {
  const target = event.target as HTMLSelectElement
  await setLayoutID(normalizedLayoutID(target.value))
}

async function setPerformanceFromEvent(event: Event) {
  const target = event.target as HTMLSelectElement
  performanceMode.value = normalizedPerformanceMode(target.value)
  if (canAdmin.value) {
    await savePerformance()
    return
  }
  await updateLayoutRoute()
}

async function setLayoutID(id: ViewerLayoutID) {
  const focus = focusSlotID()
  const custom = id === 'custom'
    ? (layoutDraft.value.id === 'custom' && layoutDraft.value.custom.cells.length ? normalizedCustomLayout(layoutDraft.value.custom) : defaultCustomLayoutForSlots(focus, focusSide.value))
    : layoutDraft.value.custom
  layoutDraft.value = {
    ...layoutDraft.value,
    id,
    mode: normalizedLayoutMode(layoutDraft.value.mode, id),
    split_percent: layoutDraft.value.split_percent || defaultSplitPercent(id),
    focus_slot_id: focus,
    custom
  }
  await applyLayoutChange()
}

function startTilePointerDrag(event: PointerEvent, slot: ViewerSlot) {
  if (!canReorderLayout.value || event.button !== 0) return
  const target = event.target
  if (target instanceof HTMLElement && target.closest('button,a,select,input')) return

  const startX = event.clientX
  const startY = event.clientY
  let active = false
  stopPointerTileDrag?.()

  const move = (moveEvent: PointerEvent) => {
    const distance = Math.hypot(moveEvent.clientX - startX, moveEvent.clientY - startY)
    if (!active && distance < 8) return
    active = true
    draggedSlotAlias.value = slot.alias
    if (activeLayoutID.value === 'custom') {
      const zone = layoutZoneFromPoint(moveEvent.clientX, moveEvent.clientY)
      dropZoneID.value = zone
      if (zone) {
        dropSlotAlias.value = ''
        moveEvent.preventDefault()
        return
      }
    }
    dropZoneID.value = ''
    const targetAlias = tileAliasFromPoint(moveEvent.clientX, moveEvent.clientY)
    dropSlotAlias.value = targetAlias && targetAlias !== slot.alias ? targetAlias : ''
    moveEvent.preventDefault()
  }
  const up = () => {
    const targetAlias = dropSlotAlias.value
    const targetZone = dropZoneID.value
    stopPointerTileDrag?.()
    stopPointerTileDrag = undefined
    if (active && activeLayoutID.value === 'custom' && targetZone) {
      void applyCustomZoneDrop(slot.alias, targetZone)
      return
    }
    if (active && targetAlias) {
      void applyTileDrop(slot.alias, targetAlias)
      return
    }
    endTileDrag()
  }
  stopPointerTileDrag = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
}

async function applyTileDrop(sourceAlias: string, targetAlias: string) {
  endTileDrag()
  if (!canAdmin.value || !sourceAlias || sourceAlias === targetAlias) return

  if (activeLayoutID.value === 'custom') {
    const custom = swapCustomCells(sourceAlias, targetAlias)
    layoutDraft.value = {
      ...layoutDraft.value,
      mode: 'custom',
      focus_slot_id: prominentCustomSlot(custom) || focusSlotID(),
      slot_order: swappedSlotOrder(sourceAlias, targetAlias),
      custom
    }
    await saveLayout()
    return
  }

  const currentFocus = focusSlotID()
  let nextFocus = currentFocus
  let nextMode = layoutDraft.value.mode
  let nextOrder = normalizedSlotOrder()
  if (splitLayout.value && sourceAlias === currentFocus) {
    nextMode = focusModeForSide(gridSideForAlias(targetAlias, currentFocus))
  } else {
    nextOrder = swappedSlotOrder(sourceAlias, targetAlias)
  }
  if ((splitLayout.value || activeLayoutID.value === 'large_only') && sourceAlias === currentFocus) {
    nextFocus = currentFocus
  } else if ((splitLayout.value || activeLayoutID.value === 'large_only') && targetAlias === currentFocus) {
    nextFocus = sourceAlias
  }

  layoutDraft.value = {
    ...layoutDraft.value,
    mode: nextMode,
    focus_slot_id: nextFocus,
    slot_order: nextOrder
  }
  await saveLayout()
}

async function applyCustomZoneDrop(sourceAlias: string, zone: DropZoneID) {
  endTileDrag()
  if (!canAdmin.value || !sourceAlias) return

  const custom = buildCustomLayoutWithLead(sourceAlias, zone)
  layoutDraft.value = {
    ...layoutDraft.value,
    id: 'custom',
    mode: 'custom',
    focus_slot_id: sourceAlias,
    slot_order: custom.cells.map((cell) => cell.slot_id),
    custom
  }
  await saveLayout()
}

function endTileDrag() {
  draggedSlotAlias.value = ''
  dropSlotAlias.value = ''
  dropZoneID.value = ''
}

async function setFocusSide(mode: 'focus_left' | 'focus_middle' | 'focus_right') {
  layoutDraft.value = { ...layoutDraft.value, mode, focus_slot_id: focusSlotID() }
  await applyLayoutChange()
}

async function applyLayoutChange() {
  if (canAdmin.value) {
    await saveLayout()
    return
  }
  await updateLayoutRoute()
}

async function updateLayoutRoute() {
  const query: Record<string, string> = {
    ...(Object.fromEntries(Object.entries(route.query).filter(([, value]) => typeof value === 'string')) as Record<string, string>),
    layout: activeLayoutID.value
  }
  if (splitLayout.value && (layoutMode.value === 'focus_left' || layoutMode.value === 'focus_right')) {
    query.side = layoutMode.value === 'focus_left' ? 'left' : 'right'
  } else if (splitLayout.value && layoutMode.value === 'focus_middle') {
    query.side = 'middle'
  } else {
    delete query.side
  }
  if (performanceMode.value !== 'quality') {
    query.perf = performanceMode.value
  } else {
    delete query.perf
  }
  await router.replace({ path: route.path, query })
}

async function savePerformance() {
  if (!canAdmin.value) return
  performanceBusy.value = true
  error.value = ''
  try {
    await api.saveSettings({
      'viewer.performance.mode': performanceMode.value
    })
    await updateLayoutRoute()
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Performance-Modus konnte nicht gespeichert werden.'
  } finally {
    performanceBusy.value = false
  }
}

async function saveLayout() {
  if (!canAdmin.value) return
  layoutBusy.value = true
  error.value = ''
  try {
    await api.saveSettings({
      'viewer.layout.id': activeLayoutID.value,
      'viewer.layout.mode': layoutDraft.value.mode,
      'viewer.layout.focus_slot_id': focusSlotID(),
      'viewer.layout.slot_order': normalizedSlotOrder().join(','),
      'viewer.layout.split_percent': String(clamp(layoutDraft.value.split_percent, 30, 76)),
      'viewer.layout.gap_px': String(clamp(layoutDraft.value.gap_px, 2, 20)),
      'viewer.layout.custom': JSON.stringify(normalizedCustomLayout(layoutDraft.value.custom))
    })
    await updateLayoutRoute()
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Layout konnte nicht gespeichert werden.'
  } finally {
    layoutBusy.value = false
  }
}

function startLayoutDrag(event: PointerEvent, handle: 'single' | 'middle_left' | 'middle_right') {
  if (!splitLayout.value || !canAdmin.value) return
  const grid = (event.currentTarget as HTMLElement).closest('.viewer-grid')
  if (!(grid instanceof HTMLElement)) return
  event.preventDefault()
  const rect = grid.getBoundingClientRect()
  const pointerId = event.pointerId
  ;(event.currentTarget as HTMLElement).setPointerCapture(pointerId)
  const move = (moveEvent: PointerEvent) => {
    const relative = clamp(Math.round(((moveEvent.clientX - rect.left) / rect.width) * 100), 20, 80)
    if (focusSide.value === 'middle') {
      const focusWidth = handle === 'middle_left'
        ? clamp((50 - relative) * 2, 24, 70)
        : clamp((relative - 50) * 2, 24, 70)
      layoutDraft.value.split_percent = clamp(100 - focusWidth, 30, 76)
      return
    }
    layoutDraft.value.split_percent = focusSide.value === 'right' ? clamp(relative, 30, 76) : clamp(100 - relative, 30, 76)
  }
  const up = async () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    await saveLayout()
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
}

function startCustomLayoutDrag(event: PointerEvent, kind: CustomGrabberKind, index: number) {
  if (activeLayoutID.value !== 'custom' || !canAdmin.value || layoutBusy.value) return
  const grid = (event.currentTarget as HTMLElement).closest('.viewer-grid')
  if (!(grid instanceof HTMLElement)) return
  event.preventDefault()
  const rect = grid.getBoundingClientRect()
  const pointerId = event.pointerId
  ;(event.currentTarget as HTMLElement).setPointerCapture(pointerId)
  const move = (moveEvent: PointerEvent) => {
    const custom = normalizedCustomLayout(layoutDraft.value.custom)
    const weights = kind === 'column' ? [...custom.columns] : [...custom.rows]
    if (index < 0 || index >= weights.length - 1) return
    const total = sum(weights)
    const pairTotal = weights[index] + weights[index + 1]
    if (pairTotal < 2 || total <= 0) return
    const axisSize = kind === 'column' ? rect.width : rect.height
    const axisPosition = kind === 'column' ? moveEvent.clientX - rect.left : moveEvent.clientY - rect.top
    const pointerWeight = clamp(Math.round((axisPosition / axisSize) * total), 0, total)
    const beforePair = sum(weights.slice(0, index))
    const minWeight = Math.max(1, Math.min(Math.round(total * 0.08), Math.floor(pairTotal / 2)))
    const nextWeight = clamp(pointerWeight - beforePair, minWeight, pairTotal - minWeight)
    weights[index] = nextWeight
    weights[index + 1] = pairTotal - nextWeight
    layoutDraft.value = {
      ...layoutDraft.value,
      custom: {
        ...custom,
        columns: kind === 'column' ? weights : custom.columns,
        rows: kind === 'row' ? weights : custom.rows
      }
    }
    moveEvent.preventDefault()
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

function sum(values: number[]) {
  return values.reduce((total, value) => total + value, 0)
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

watch(
  () => [route.query.layout, route.query.side, route.query.perf],
  () => {
    syncLayoutDraft()
    syncPerformanceDraft()
  }
)

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
  stopPointerTileDrag?.()
  window.clearInterval(refreshTimer)
  if (onAuthChanged) window.removeEventListener('auth-changed', onAuthChanged)
})
</script>
