<template>
  <div
    class="viewer-root"
    :class="rootClass"
    @pointermove="revealControls"
    @mouseleave="onRootMouseLeave"
    @selectstart.prevent
  >
    <section ref="gridEl" class="mosaic">
      <div
        v-for="pane in panes"
        :key="pane.alias"
        class="mosaic-pane"
        :style="paneStyle(pane)"
      >
        <article
          class="viewer-tile"
          :class="[tileClass(pane.slot), { dragging: dragSourceAlias === pane.alias, audible: isAudible(pane.alias) }]"
          :data-slot-alias="pane.alias"
        >
          <div class="viewer-frame-wrap">
            <div
              v-if="shouldRenderPlayer(pane.slot)"
              class="viewer-frame-transform"
              :class="displayClass(pane.slot)"
              :style="displayStyle(pane.slot)"
            >
              <iframe
                class="viewer-frame"
                :ref="(el) => setFrameRef(pane.alias, el)"
                :src="frameSrc(pane.slot)"
                :title="pane.slot.label"
                :loading="iframeLoading(pane.slot)"
                allow="autoplay; fullscreen; picture-in-picture"
                @load="markFrameReady(pane.alias)"
              />
            </div>
            <div
              v-if="shouldRenderHDPlayer(pane.slot)"
              class="viewer-frame-transform viewer-frame-transform-hd"
              :class="[displayClass(pane.slot), { ready: isHDFrameReady(pane.alias) }]"
              :style="displayStyle(pane.slot)"
            >
              <iframe
                class="viewer-frame"
                :src="hdFrameSrc(pane.slot)"
                :title="`${pane.slot.label} HD`"
                loading="eager"
                allow="autoplay; fullscreen; picture-in-picture"
                @load="markHDFrameReady(pane.alias)"
              />
            </div>
            <div v-if="!shouldRenderPlayer(pane.slot)" class="viewer-placeholder" :class="{ paused: isPausedByPerformance(pane.slot) }">
              <div class="placeholder-mark">{{ pane.alias }}</div>
              <div>{{ placeholderMessage(pane.slot) }}</div>
            </div>
            <div v-if="isPausedByPerformance(pane.slot)" class="viewer-cover performance-cover">
              <span>Standby</span>
            </div>
            <div v-if="effectiveState(pane.slot) === 'connecting'" class="viewer-cover">
              <span class="loader-dot" />
              <span>Verbindet</span>
            </div>
          </div>

          <div
            class="tile-surface"
            @click="onTileClick(pane.alias)"
            @dblclick="onTileDoubleClick(pane.alias)"
            @pointerdown="onTilePointerDown($event, pane.slot)"
            @wheel="onTileWheel($event, pane.slot)"
          />

          <div v-if="editing" class="tile-edit">
            <span class="tile-tag">{{ pane.alias }} · {{ pane.slot.label }}</span>
            <div v-if="pane.slot.binding?.device_id" class="tile-edit-actions">
              <button class="btn icon sm" type="button" title="90° drehen" @click.stop="rotateTile(pane.slot)">⟳</button>
              <button
                class="btn sm"
                type="button"
                :title="effectiveDisplay(pane.slot).fit_mode === 'cover' ? 'Ganzes Bild zeigen' : 'Format füllen'"
                @click.stop="toggleFitTile(pane.slot)"
              >{{ effectiveDisplay(pane.slot).fit_mode === 'cover' ? 'Füllen' : 'Ganz' }}</button>
              <button class="btn icon sm" type="button" title="Hineinzoomen" @click.stop="zoomTile(pane.slot, -1)">＋</button>
              <button class="btn icon sm" type="button" title="Herauszoomen" @click.stop="zoomTile(pane.slot, 1)">－</button>
              <button class="btn icon sm" type="button" title="Zuschnitt zurücksetzen" @click.stop="resetTile(pane.slot)">⟲</button>
            </div>
          </div>
        </article>
      </div>

      <template v-if="editing && !spotlightAlias">
        <button
          v-for="gutter in gutters"
          :key="gutter.id"
          class="mosaic-gutter"
          :class="gutter.dir"
          type="button"
          :style="gutterStyle(gutter)"
          :title="gutter.dir === 'row' ? 'Breite ziehen' : 'Höhe ziehen'"
          @pointerdown="startGutterDrag($event, gutter)"
        />
      </template>

      <div v-if="dockTarget" class="mosaic-dock" :style="dockOverlayStyle" aria-hidden="true" />

      <div v-if="!panes.length" class="viewer-empty">
        <div>Noch keine Kamera ausgewählt.</div>
        <RouterLink v-if="canAdmin" class="btn sm" to="/einrichtung">Kameras aktivieren</RouterLink>
      </div>
    </section>

    <transition name="hud">
      <div v-if="showHud" class="viewer-hud" @pointermove.stop="revealControls" @click.stop>
        <template v-if="canAdmin">
          <button class="btn sm" :class="{ live: editing }" type="button" @click="toggleEdit">{{ editing ? 'Fertig' : 'Bearbeiten' }}</button>
        </template>
        <button class="btn sm" type="button" @click="toggleFullscreen">{{ isFullscreen ? 'Vollbild aus' : 'Vollbild' }}</button>
        <button
          class="btn icon sm audio-toggle"
          :class="{ live: audioEnabled, ghost: !audioEnabled }"
          type="button"
          :aria-label="audioToggleTitle"
          :aria-pressed="audioEnabled"
          @click="toggleAudioEnabled"
        >
          <svg v-if="audioEnabled" viewBox="0 0 24 24" aria-hidden="true">
            <path d="M4 9v6h4l5 4V5L8 9H4Z" />
            <path d="M16 8.5a5 5 0 0 1 0 7" />
            <path d="M18.5 6a8 8 0 0 1 0 12" />
          </svg>
          <svg v-else viewBox="0 0 24 24" aria-hidden="true">
            <path d="M4 9v6h4l5 4V5L8 9H4Z" />
            <path d="m17 9 5 5" />
            <path d="m22 9-5 5" />
          </svg>
        </button>
        <RouterLink v-if="canAdmin" class="btn sm ghost" to="/einrichtung">Verwaltung</RouterLink>
        <RouterLink v-else-if="auth?.enabled && !auth.authenticated" class="btn sm ghost" to="/login">Login</RouterLink>
      </div>
    </transition>

    <transition name="hud">
      <div v-if="error" class="viewer-error" @click="error = ''">{{ error }}</div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import type { CSSProperties } from 'vue'
import { api } from '../api/client'
import type { AuthStatus, CameraDisplay, ViewerResponse, ViewerSlot, ViewerSlotState } from '../types'

// --- Mosaic layout model -----------------------------------------------------
// A binary split tree, like VSCode editor groups. A leaf shows one camera; a
// split divides its area into two children (row = side by side, col = stacked)
// with a ratio for the first child. Cameras dock onto a pane's edge to create a
// new split, or onto the centre to swap.
type MosaicLeaf = { type: 'leaf'; slot: string }
type MosaicSplit = { type: 'split'; dir: 'row' | 'col'; ratio: number; a: MosaicNode; b: MosaicNode }
type MosaicNode = MosaicLeaf | MosaicSplit
type Rect = { x: number; y: number; w: number; h: number }
type Side = 'left' | 'right' | 'top' | 'bottom' | 'center'
type PaneRect = { alias: string; rect: Rect }
type GutterRect = { id: string; path: string; dir: 'row' | 'col'; rect: Rect; line: Rect }

const viewer = ref<ViewerResponse>()
const auth = ref<AuthStatus>()
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const frameReady = ref<Record<string, boolean>>({})
const hdFrameReady = ref<Record<string, boolean>>({})
const performanceMode = ref<'quality' | 'balanced' | 'low' | 'diagnostic'>('quality')
const audioEnabled = ref(true)
const activeAudioAlias = ref('')

// Chrome state: clean by default; edit reveals split tools; spotlight enlarges a
// single camera; fullscreen suppresses all chrome.
const editing = ref(false)
const spotlightAlias = ref('')
const controlsVisible = ref(false)
const isFullscreen = ref(false)
const displayOverrides = ref<Record<string, CameraDisplay>>({})

// Layout tree + live drag state.
const mosaic = ref<MosaicNode>()
const gridEl = ref<HTMLElement>()
const dragSourceAlias = ref('')
const dockTarget = ref('')
const dockSide = ref<Side>('center')
const ROOT_TARGET = '__root__'

let refreshTimer = 0
let controlsTimer = 0
let tileClickTimer = 0
let displaySaveTimer = 0
let mosaicSaveTimer = 0
let onAuthChanged: (() => void) | undefined
let onFullscreenChange: (() => void) | undefined
let onKey: ((e: KeyboardEvent) => void) | undefined
let stopDrag: (() => void) | undefined
let stopCropPan: (() => void) | undefined
const frameEls: Record<string, HTMLIFrameElement> = {}
const AUDIO_SETTING_KEY = 'camera-appliance.viewer.audioEnabled'
const LEGACY_AUDIO_SETTING_KEY = 'camera-appliance.viewer.audioHoverEnabled'

const slots = computed(() => viewer.value?.slots ?? [])
const slotByAlias = computed(() => new Map(slots.value.map((slot) => [slot.alias, slot])))
const canAdmin = computed(() => (auth.value ? !auth.value.enabled || auth.value.role === 'admin' : false))

const rootClass = computed(() => ({
  editing: editing.value,
  spotlight: !!spotlightAlias.value,
  fullscreen: isFullscreen.value,
  docking: !!dragSourceAlias.value
}))
const showHud = computed(() => !isFullscreen.value && (controlsVisible.value || editing.value))

// --- Tree geometry -----------------------------------------------------------

function computeRects(node: MosaicNode, rect: Rect, path: string, leaves: PaneRect[], gutters: GutterRect[]) {
  if (node.type === 'leaf') {
    leaves.push({ alias: node.slot, rect })
    return
  }
  const ratio = clamp(node.ratio, 0.05, 0.95)
  if (node.dir === 'row') {
    const aw = rect.w * ratio
    const aRect = { x: rect.x, y: rect.y, w: aw, h: rect.h }
    const bRect = { x: rect.x + aw, y: rect.y, w: rect.w - aw, h: rect.h }
    gutters.push({ id: path, path, dir: 'row', rect, line: { x: rect.x + aw, y: rect.y, w: 0, h: rect.h } })
    computeRects(node.a, aRect, path + 'a', leaves, gutters)
    computeRects(node.b, bRect, path + 'b', leaves, gutters)
  } else {
    const ah = rect.h * ratio
    const aRect = { x: rect.x, y: rect.y, w: rect.w, h: ah }
    const bRect = { x: rect.x, y: rect.y + ah, w: rect.w, h: rect.h - ah }
    gutters.push({ id: path, path, dir: 'col', rect, line: { x: rect.x, y: rect.y + ah, w: rect.w, h: 0 } })
    computeRects(node.a, aRect, path + 'a', leaves, gutters)
    computeRects(node.b, bRect, path + 'b', leaves, gutters)
  }
}

const geometry = computed(() => {
  const leaves: PaneRect[] = []
  const gutters: GutterRect[] = []
  if (mosaic.value) computeRects(mosaic.value, { x: 0, y: 0, w: 100, h: 100 }, '', leaves, gutters)
  return { leaves, gutters }
})

const panes = computed(() => {
  const map = slotByAlias.value
  return geometry.value.leaves
    .filter((leaf) => map.has(leaf.alias))
    .map((leaf) => ({ alias: leaf.alias, slot: map.get(leaf.alias) as ViewerSlot, rect: leaf.rect }))
})
const gutters = computed(() => geometry.value.gutters)
const audioToggleTitle = computed(() => audioEnabled.value ? 'Ton global freigegeben' : 'Ton global aus')

const primaryLiveAlias = computed(() => {
  if (spotlightAlias.value) return spotlightAlias.value
  let alias = ''
  let area = 0
  for (const leaf of geometry.value.leaves) {
    const size = leaf.rect.w * leaf.rect.h
    if (size > area) {
      area = size
      alias = leaf.alias
    }
  }
  return alias
})

function paneStyle(pane: PaneRect): CSSProperties {
  const spotlight = spotlightAlias.value
  const isSpotlight = spotlight === pane.alias
  const rect = isSpotlight ? { x: 0, y: 0, w: 100, h: 100 } : pane.rect
  return {
    left: `${rect.x}%`,
    top: `${rect.y}%`,
    width: `${rect.w}%`,
    height: `${rect.h}%`,
    zIndex: spotlight ? (isSpotlight ? 5 : 1) : undefined,
    pointerEvents: spotlight && !isSpotlight ? 'none' : undefined
  }
}

function gutterStyle(gutter: GutterRect) {
  if (gutter.dir === 'row') {
    return { left: `${gutter.line.x}%`, top: `${gutter.rect.y}%`, height: `${gutter.rect.h}%` }
  }
  return { top: `${gutter.line.y}%`, left: `${gutter.rect.x}%`, width: `${gutter.rect.w}%` }
}

const dockOverlayStyle = computed(() => {
  const side = dockSide.value
  const base = dockTarget.value === ROOT_TARGET
    ? { x: 0, y: 0, w: 100, h: 100 }
    : geometry.value.leaves.find((item) => item.alias === dockTarget.value)?.rect
  if (!base) return { display: 'none' }
  const { x, y, w, h } = base
  let rect = { x, y, w, h }
  if (side === 'left') rect = { x, y, w: w / 2, h }
  else if (side === 'right') rect = { x: x + w / 2, y, w: w / 2, h }
  else if (side === 'top') rect = { x, y, w, h: h / 2 }
  else if (side === 'bottom') rect = { x, y: y + h / 2, w, h: h / 2 }
  return { left: `${rect.x}%`, top: `${rect.y}%`, width: `${rect.w}%`, height: `${rect.h}%`, display: 'block' }
})

// --- Tree transforms (immutable) --------------------------------------------

function leaf(slot: string): MosaicLeaf {
  return { type: 'leaf', slot }
}

function treeSlots(node: MosaicNode | undefined, out: string[] = []): string[] {
  if (!node) return out
  if (node.type === 'leaf') out.push(node.slot)
  else {
    treeSlots(node.a, out)
    treeSlots(node.b, out)
  }
  return out
}

// Default = an even grid: columns = ceil(sqrt(n)) (4 → 2x2, 9 → 3x3, 5 → 3+2),
// rows filled as evenly as possible, all cells equal size.
function gridTree(aliases: string[]): MosaicNode {
  if (aliases.length <= 1) return leaf(aliases[0] || 'cam1')
  const n = aliases.length
  const cols = Math.ceil(Math.sqrt(n))
  const rows = Math.ceil(n / cols)
  const base = Math.floor(n / rows)
  const extra = n % rows
  const rowNodes: MosaicNode[] = []
  let index = 0
  for (let row = 0; row < rows; row += 1) {
    const count = base + (row < extra ? 1 : 0)
    const cells = aliases.slice(index, index + count).map((alias) => leaf(alias))
    index += count
    if (cells.length) rowNodes.push(evenSplit(cells, 'row'))
  }
  return evenSplit(rowNodes, 'col')
}

// Build a left-leaning split tree whose leaves all get an equal share.
function evenSplit(nodes: MosaicNode[], dir: 'row' | 'col'): MosaicNode {
  if (nodes.length === 1) return nodes[0]
  return { type: 'split', dir, ratio: 1 / nodes.length, a: nodes[0], b: evenSplit(nodes.slice(1), dir) }
}

function removeSlot(node: MosaicNode, slot: string): MosaicNode | null {
  if (node.type === 'leaf') return node.slot === slot ? null : node
  const a = removeSlot(node.a, slot)
  const b = removeSlot(node.b, slot)
  if (a === null) return b
  if (b === null) return a
  return { ...node, a, b }
}

function splitAtLeaf(node: MosaicNode, targetSlot: string, source: MosaicLeaf, side: Side): MosaicNode {
  if (node.type === 'leaf') {
    if (node.slot !== targetSlot) return node
    if (side === 'left') return { type: 'split', dir: 'row', ratio: 0.5, a: source, b: node }
    if (side === 'right') return { type: 'split', dir: 'row', ratio: 0.5, a: node, b: source }
    if (side === 'top') return { type: 'split', dir: 'col', ratio: 0.5, a: source, b: node }
    if (side === 'bottom') return { type: 'split', dir: 'col', ratio: 0.5, a: node, b: source }
    return node
  }
  return { ...node, a: splitAtLeaf(node.a, targetSlot, source, side), b: splitAtLeaf(node.b, targetSlot, source, side) }
}

function swapSlots(node: MosaicNode, a: string, b: string): MosaicNode {
  if (node.type === 'leaf') {
    if (node.slot === a) return leaf(b)
    if (node.slot === b) return leaf(a)
    return node
  }
  return { ...node, a: swapSlots(node.a, a, b), b: swapSlots(node.b, a, b) }
}

function setRatioAtPath(node: MosaicNode, path: string, ratio: number): MosaicNode {
  if (path === '') {
    return node.type === 'split' ? { ...node, ratio: clamp(ratio, 0.05, 0.95) } : node
  }
  if (node.type !== 'split') return node
  const head = path[0]
  const rest = path.slice(1)
  if (head === 'a') return { ...node, a: setRatioAtPath(node.a, rest, ratio) }
  return { ...node, b: setRatioAtPath(node.b, rest, ratio) }
}

function reconcileTree(tree: MosaicNode | undefined, aliases: string[]): MosaicNode {
  if (!aliases.length) return leaf('cam1')
  let next: MosaicNode | undefined = tree
  for (const slot of treeSlots(next)) {
    if (!aliases.includes(slot)) {
      next = next ? removeSlot(next, slot) ?? undefined : undefined
    }
  }
  for (const alias of aliases) {
    if (!treeSlots(next).includes(alias)) {
      next = next ? { type: 'split', dir: 'row', ratio: 0.7, a: next, b: leaf(alias) } : leaf(alias)
    }
  }
  return next ?? gridTree(aliases)
}

// --- Mosaic interactions -----------------------------------------------------

function setTree(node: MosaicNode) {
  mosaic.value = node
  scheduleMosaicSave()
}

function onTileClick(alias: string) {
  if (editing.value) return
  if (!audioEnabled.value) {
    toggleSpotlight(alias)
    return
  }
  window.clearTimeout(tileClickTimer)
  tileClickTimer = window.setTimeout(() => {
    activeAudioAlias.value = activeAudioAlias.value === alias ? '' : alias
    syncAudioState()
  }, 220)
}

function onTileDoubleClick(alias: string) {
  if (editing.value) return
  window.clearTimeout(tileClickTimer)
  toggleSpotlight(alias)
}

function onTilePointerDown(event: PointerEvent, slot: ViewerSlot) {
  if (!editing.value || event.button !== 0 || spotlightAlias.value) return
  const target = event.target
  if (target instanceof HTMLElement && target.closest('button,a,select,input')) return
  if (event.shiftKey && slot.binding?.device_id) {
    startCropPan(event, slot)
  } else {
    startPaneDrag(event, slot.alias)
  }
}

function onTileWheel(event: WheelEvent, slot: ViewerSlot) {
  if (!editing.value || !slot.binding?.device_id) return
  event.preventDefault()
  zoomTile(slot, event.deltaY > 0 ? 1 : -1)
}

function startPaneDrag(event: PointerEvent, alias: string) {
  if (treeSlots(mosaic.value).length < 2) return
  const startX = event.clientX
  const startY = event.clientY
  let active = false
  stopDrag?.()
  event.preventDefault()

  const move = (moveEvent: PointerEvent) => {
    if (!active && Math.hypot(moveEvent.clientX - startX, moveEvent.clientY - startY) < 8) return
    active = true
    dragSourceAlias.value = alias
    const hit = hitTest(moveEvent.clientX, moveEvent.clientY)
    dockTarget.value = hit && hit.alias !== alias ? hit.alias : ''
    dockSide.value = hit?.side ?? 'center'
    moveEvent.preventDefault()
  }
  const up = () => {
    const targetAlias = dockTarget.value
    const side = dockSide.value
    const wasActive = active
    stopDrag?.()
    stopDrag = undefined
    endPaneDrag()
    if (wasActive && targetAlias) applyDock(alias, targetAlias, side)
  }
  stopDrag = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
}

function endPaneDrag() {
  dragSourceAlias.value = ''
  dockTarget.value = ''
  dockSide.value = 'center'
}

function hitTest(clientX: number, clientY: number): { alias: string; side: Side } | undefined {
  const el = gridEl.value
  if (!el) return undefined
  const box = el.getBoundingClientRect()
  const px = ((clientX - box.left) / box.width) * 100
  const py = ((clientY - box.top) / box.height) * 100
  // Outer edge of the whole layout → dock as a full-width/height pane (split the root),
  // like dropping on the edge of the VSCode editor area.
  const outerEdges: Array<{ side: Side; d: number }> = [
    { side: 'left', d: px },
    { side: 'right', d: 100 - px },
    { side: 'top', d: py },
    { side: 'bottom', d: 100 - py }
  ]
  outerEdges.sort((a, b) => a.d - b.d)
  if (outerEdges[0].d <= 7) return { alias: ROOT_TARGET, side: outerEdges[0].side }

  const target = geometry.value.leaves.find((item) =>
    px >= item.rect.x && px <= item.rect.x + item.rect.w && py >= item.rect.y && py <= item.rect.y + item.rect.h
  )
  if (!target) return undefined
  const lx = (px - target.rect.x) / target.rect.w
  const ly = (py - target.rect.y) / target.rect.h
  const distances: Array<{ side: Side; d: number }> = [
    { side: 'left', d: lx },
    { side: 'right', d: 1 - lx },
    { side: 'top', d: ly },
    { side: 'bottom', d: 1 - ly }
  ]
  distances.sort((a, b) => a.d - b.d)
  const side: Side = distances[0].d < 0.28 ? distances[0].side : 'center'
  return { alias: target.alias, side }
}

function applyDock(sourceAlias: string, targetAlias: string, side: Side) {
  if (!mosaic.value || sourceAlias === targetAlias) return
  // Dock to the outer edge of the whole layout → wrap the entire tree in a new split,
  // giving the camera a full-height column (left/right) or full-width row (top/bottom).
  if (targetAlias === ROOT_TARGET) {
    if (side === 'center') return
    const rest = removeSlot(mosaic.value, sourceAlias)
    if (!rest) return
    const src = leaf(sourceAlias)
    const dir: 'row' | 'col' = side === 'left' || side === 'right' ? 'row' : 'col'
    const sourceFirst = side === 'left' || side === 'top'
    setTree({ type: 'split', dir, ratio: 0.5, a: sourceFirst ? src : rest, b: sourceFirst ? rest : src })
    return
  }
  if (side === 'center') {
    setTree(swapSlots(mosaic.value, sourceAlias, targetAlias))
    return
  }
  const withoutSource = removeSlot(mosaic.value, sourceAlias)
  if (!withoutSource) return
  setTree(splitAtLeaf(withoutSource, targetAlias, leaf(sourceAlias), side))
}

function startGutterDrag(event: PointerEvent, gutter: GutterRect) {
  const el = gridEl.value
  if (!el || !mosaic.value) return
  event.preventDefault()
  stopDrag?.()
  const box = el.getBoundingClientRect()
  const move = (moveEvent: PointerEvent) => {
    if (!mosaic.value) return
    let ratio: number
    if (gutter.dir === 'row') {
      const px = ((moveEvent.clientX - box.left) / box.width) * 100
      ratio = (px - gutter.rect.x) / gutter.rect.w
    } else {
      const py = ((moveEvent.clientY - box.top) / box.height) * 100
      ratio = (py - gutter.rect.y) / gutter.rect.h
    }
    mosaic.value = setRatioAtPath(mosaic.value, gutter.path, ratio)
    moveEvent.preventDefault()
  }
  const up = () => {
    stopDrag?.()
    stopDrag = undefined
    scheduleMosaicSave()
  }
  stopDrag = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
}

function scheduleMosaicSave() {
  if (!canAdmin.value) return
  window.clearTimeout(mosaicSaveTimer)
  mosaicSaveTimer = window.setTimeout(() => void saveMosaic(), 400)
}

async function saveMosaic() {
  if (!canAdmin.value || !mosaic.value) return
  try {
    await api.saveSettings({ 'viewer.layout.mosaic': JSON.stringify(mosaic.value) })
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Layout konnte nicht gespeichert werden.'
  }
}

function parseMosaic(raw: string | undefined): MosaicNode | undefined {
  if (!raw) return undefined
  try {
    return normalizeNode(JSON.parse(raw))
  } catch {
    return undefined
  }
}

function normalizeNode(value: unknown): MosaicNode | undefined {
  if (!value || typeof value !== 'object') return undefined
  const node = value as Record<string, unknown>
  if (node.type === 'leaf' && typeof node.slot === 'string') return leaf(node.slot)
  if (node.type === 'split' && (node.dir === 'row' || node.dir === 'col')) {
    const a = normalizeNode(node.a)
    const b = normalizeNode(node.b)
    if (!a || !b) return undefined
    const ratio = typeof node.ratio === 'number' ? clamp(node.ratio, 0.05, 0.95) : 0.5
    return { type: 'split', dir: node.dir, ratio, a, b }
  }
  return undefined
}

// --- Chrome: controls auto-hide, spotlight, edit, fullscreen -----------------

function revealControls() {
  controlsVisible.value = true
  window.clearTimeout(controlsTimer)
  if (editing.value) return
  controlsTimer = window.setTimeout(() => {
    controlsVisible.value = false
  }, 2600)
}

function scheduleHideControls() {
  if (editing.value) return
  window.clearTimeout(controlsTimer)
  controlsTimer = window.setTimeout(() => {
    controlsVisible.value = false
  }, 600)
}

function onRootMouseLeave() {
  scheduleHideControls()
}

function toggleEdit() {
  editing.value = !editing.value
  spotlightAlias.value = ''
  controlsVisible.value = true
  if (!editing.value) window.clearTimeout(controlsTimer)
}

function toggleSpotlight(alias: string) {
  const next = spotlightAlias.value === alias ? '' : alias
  spotlightAlias.value = next
  if (next) hdFrameReady.value = { ...hdFrameReady.value, [next]: false }
}

async function toggleFullscreen() {
  try {
    if (document.fullscreenElement) await document.exitFullscreen()
    else await document.documentElement.requestFullscreen()
  } catch {
    isFullscreen.value = !isFullscreen.value
  }
}

function syncFullscreen() {
  isFullscreen.value = !!document.fullscreenElement
}

// --- Audio -------------------------------------------------------------------

function setFrameRef(alias: string, el: unknown) {
  if (el instanceof HTMLIFrameElement) {
    frameEls[alias] = el
    window.setTimeout(() => syncAudioState(), 0)
  } else {
    delete frameEls[alias]
  }
}

function isAudible(alias: string) {
  return audioEnabled.value && activeAudioAlias.value === alias
}

function toggleAudioEnabled() {
  audioEnabled.value = !audioEnabled.value
  window.localStorage.setItem(AUDIO_SETTING_KEY, audioEnabled.value ? 'true' : 'false')
  if (!audioEnabled.value) activeAudioAlias.value = ''
  syncAudioState()
}

function muteAllAudio() {
  activeAudioAlias.value = ''
  syncAudioState()
}

function syncAudioState() {
  for (const [alias, frame] of Object.entries(frameEls)) {
    frame.contentWindow?.postMessage({
      type: 'camera-audio',
      muted: !audioEnabled.value || alias !== activeAudioAlias.value
    }, window.location.origin)
  }
}

// --- Camera display (transform + inline crop) --------------------------------

function effectiveDisplay(slot: ViewerSlot): CameraDisplay {
  return displayOverrides.value[slot.alias] ?? normalizedDisplay(slot.display)
}

function displayClass(slot: ViewerSlot) {
  const display = effectiveDisplay(slot)
  return {
    'fit-contain': display.fit_mode === 'contain',
    'rotated-quarter': display.rotation === 90 || display.rotation === 270
  }
}

function displayStyle(slot: ViewerSlot) {
  const display = effectiveDisplay(slot)
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
    fit_mode: display?.fit_mode === 'cover' ? 'cover' : 'contain',
    crop: {
      x: clamp(display?.crop?.x ?? 0, 0, 99),
      y: clamp(display?.crop?.y ?? 0, 0, 99),
      width: clamp(display?.crop?.width ?? 100, 1, 100),
      height: clamp(display?.crop?.height ?? 100, 1, 100)
    }
  }
}

function setDisplayOverride(slot: ViewerSlot, display: CameraDisplay) {
  displayOverrides.value = { ...displayOverrides.value, [slot.alias]: display }
}

function rotateTile(slot: ViewerSlot) {
  const display = effectiveDisplay(slot)
  const rotation = (((display.rotation || 0) + 90) % 360) as CameraDisplay['rotation']
  setDisplayOverride(slot, { ...display, rotation })
  scheduleDisplaySave(slot)
}

function toggleFitTile(slot: ViewerSlot) {
  const display = effectiveDisplay(slot)
  const fit_mode = display.fit_mode === 'cover' ? 'contain' : 'cover'
  setDisplayOverride(slot, { ...display, fit_mode })
  scheduleDisplaySave(slot)
}

function zoomTile(slot: ViewerSlot, direction: number) {
  const display = effectiveDisplay(slot)
  const step = 8
  const width = clamp(display.crop.width + direction * step, 20, 100)
  const height = clamp(display.crop.height + direction * step, 20, 100)
  const centerX = display.crop.x + display.crop.width / 2
  const centerY = display.crop.y + display.crop.height / 2
  const crop = {
    x: clamp(centerX - width / 2, 0, 100 - width),
    y: clamp(centerY - height / 2, 0, 100 - height),
    width,
    height
  }
  setDisplayOverride(slot, { ...display, fit_mode: 'cover', crop })
  scheduleDisplaySave(slot)
}

function resetTile(slot: ViewerSlot) {
  setDisplayOverride(slot, {
    rotation: 0,
    mirror: false,
    flip: false,
    fit_mode: 'contain',
    crop: { x: 0, y: 0, width: 100, height: 100 }
  })
  scheduleDisplaySave(slot)
}

function startCropPan(event: PointerEvent, slot: ViewerSlot) {
  const base = effectiveDisplay(slot)
  if (base.crop.width >= 100 && base.crop.height >= 100) return
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const startX = event.clientX
  const startY = event.clientY
  let moved = false
  event.preventDefault()
  stopCropPan?.()
  const move = (moveEvent: PointerEvent) => {
    moved = true
    const dx = ((moveEvent.clientX - startX) / rect.width) * base.crop.width
    const dy = ((moveEvent.clientY - startY) / rect.height) * base.crop.height
    const crop = {
      ...base.crop,
      x: clamp(base.crop.x - dx, 0, 100 - base.crop.width),
      y: clamp(base.crop.y - dy, 0, 100 - base.crop.height)
    }
    setDisplayOverride(slot, { ...base, crop })
    moveEvent.preventDefault()
  }
  const up = () => {
    stopCropPan?.()
    stopCropPan = undefined
    if (moved) scheduleDisplaySave(slot)
  }
  stopCropPan = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
}

function scheduleDisplaySave(slot: ViewerSlot) {
  if (!canAdmin.value) return
  window.clearTimeout(displaySaveTimer)
  displaySaveTimer = window.setTimeout(() => void persistDisplay(slot), 500)
}

async function persistDisplay(slot: ViewerSlot) {
  const deviceID = slot.binding?.device_id
  const display = displayOverrides.value[slot.alias]
  if (!deviceID || !display) return
  try {
    await api.saveSettings({
      [`camera.display.${deviceID}.rotation`]: String(display.rotation),
      [`camera.display.${deviceID}.mirror`]: String(display.mirror),
      [`camera.display.${deviceID}.flip`]: String(display.flip),
      [`camera.display.${deviceID}.fit_mode`]: display.fit_mode,
      [`camera.display.${deviceID}.crop_x`]: String(Math.round(display.crop.x)),
      [`camera.display.${deviceID}.crop_y`]: String(Math.round(display.crop.y)),
      [`camera.display.${deviceID}.crop_width`]: String(Math.round(display.crop.width)),
      [`camera.display.${deviceID}.crop_height`]: String(Math.round(display.crop.height))
    })
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Anzeige konnte nicht gespeichert werden.'
  }
}

// --- Stream / state helpers --------------------------------------------------

function isPlayable(slot: ViewerSlot) {
  return slot.state === 'online' || slot.state === 'connecting'
}

function shouldRenderPlayer(slot: ViewerSlot) {
  if (!slot.playback?.page_url || !isPlayable(slot)) return false
  if (performanceMode.value === 'low') return slot.alias === primaryLiveAlias.value
  return true
}

function shouldRenderHDPlayer(slot: ViewerSlot) {
  if (spotlightAlias.value !== slot.alias || !slot.playback?.hd_page_url || !isPlayable(slot)) return false
  return slot.playback.hd_page_url !== slot.playback.page_url
}

function isPausedByPerformance(slot: ViewerSlot) {
  return performanceMode.value === 'low' && !!slot.playback?.page_url && isPlayable(slot) && slot.alias !== primaryLiveAlias.value
}

function iframeLoading(slot: ViewerSlot): 'eager' | 'lazy' {
  if (performanceMode.value === 'balanced' && slot.alias !== primaryLiveAlias.value) return 'lazy'
  return 'eager'
}

function frameSrc(slot: ViewerSlot) {
  const url = slot.playback?.page_url
  if (!url) return ''
  return `${url}&fit=${effectiveDisplay(slot).fit_mode}`
}

function hdFrameSrc(slot: ViewerSlot) {
  const url = slot.playback?.hd_page_url
  if (!url) return ''
  return `${url}&fit=${effectiveDisplay(slot).fit_mode}`
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
  syncAudioState()
}

function markHDFrameReady(alias: string) {
  hdFrameReady.value = { ...hdFrameReady.value, [alias]: true }
}

function isHDFrameReady(alias: string) {
  return hdFrameReady.value[alias] === true
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

function clamp(value: number, min: number, max: number) {
  if (!Number.isFinite(value)) return min
  return Math.min(max, Math.max(min, value))
}

// --- Loading -----------------------------------------------------------------

async function load() {
  busy.value = true
  error.value = ''
  try {
    const viewerData = await api.viewer()
    viewer.value = viewerData
    performanceMode.value = normalizedPerformanceMode(viewerData.performance?.mode)
    // Only cameras that are activated (have a binding) appear in the viewer; their
    // placement is controlled here, slots are assigned automatically in the background.
    const aliases = viewerData.slots.filter((slot) => slot.binding?.device_id).map((slot) => slot.alias)
    mosaic.value = aliases.length ? reconcileTree(parseMosaic(viewerData.layout?.mosaic), aliases) : undefined
    if (spotlightAlias.value && !aliases.includes(spotlightAlias.value)) spotlightAlias.value = ''
    if (activeAudioAlias.value && !aliases.includes(activeAudioAlias.value)) muteAllAudio()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Viewer konnte nicht geladen werden.'
  } finally {
    loading.value = false
    busy.value = false
  }
}

function normalizedPerformanceMode(raw?: string): 'quality' | 'balanced' | 'low' | 'diagnostic' {
  if (raw === 'balanced' || raw === 'low' || raw === 'diagnostic') return raw
  return 'quality'
}

async function refreshAuth() {
  try {
    auth.value = await api.authStatus()
  } catch {
    auth.value = undefined
  }
}

onMounted(() => {
  audioEnabled.value = (window.localStorage.getItem(AUDIO_SETTING_KEY) ?? window.localStorage.getItem(LEGACY_AUDIO_SETTING_KEY)) !== 'false'
  void refreshAuth()
  onAuthChanged = () => void refreshAuth()
  window.addEventListener('auth-changed', onAuthChanged)
  onFullscreenChange = () => syncFullscreen()
  document.addEventListener('fullscreenchange', onFullscreenChange)
  onKey = (event: KeyboardEvent) => {
    if (event.key !== 'Escape') return
    if (spotlightAlias.value) spotlightAlias.value = ''
    else if (editing.value) editing.value = false
  }
  window.addEventListener('keydown', onKey)
  void load()
  refreshTimer = window.setInterval(() => {
    if (!busy.value && !editing.value && !spotlightAlias.value) void load()
  }, 15000)
})

onBeforeUnmount(() => {
  muteAllAudio()
  stopDrag?.()
  stopCropPan?.()
  window.clearInterval(refreshTimer)
  window.clearTimeout(controlsTimer)
  window.clearTimeout(tileClickTimer)
  window.clearTimeout(displaySaveTimer)
  window.clearTimeout(mosaicSaveTimer)
  if (onAuthChanged) window.removeEventListener('auth-changed', onAuthChanged)
  if (onFullscreenChange) document.removeEventListener('fullscreenchange', onFullscreenChange)
  if (onKey) window.removeEventListener('keydown', onKey)
})
</script>

<style scoped>
/* Full-bleed kiosk surface: nothing but cameras by default. */
.viewer-root {
  position: relative;
  min-height: 100vh;
  height: 100vh;
  background: var(--bg);
  overflow: hidden;
  user-select: none;
  -webkit-user-select: none;
}

.mosaic {
  position: absolute;
  inset: 9px;
}
.viewer-root.fullscreen .mosaic { inset: 0; }

.mosaic-pane {
  position: absolute;
}
.viewer-empty {
  position: absolute;
  inset: 0;
  display: grid;
  place-content: center;
  justify-items: center;
  gap: 14px;
  color: var(--ink-mute);
  font-size: 13px;
  letter-spacing: .02em;
}
.mosaic-pane > .viewer-tile {
  position: absolute;
  inset: 5px;
  min-height: 0;
}
.viewer-root.editing .mosaic-pane > .viewer-tile {
  box-shadow: inset 0 0 0 1px var(--hairline-strong);
}
.viewer-tile.dragging { opacity: .5; }
.viewer-tile.audible {
  box-shadow:
    inset 0 0 0 2px var(--live),
    0 0 0 1px rgba(181, 232, 83, .25),
    0 0 22px rgba(181, 232, 83, .22);
}

/* transparent layer above the iframe so the tile is clickable/draggable */
.tile-surface {
  position: absolute;
  inset: 0;
  z-index: 2;
}
.viewer-root:not(.editing) .tile-surface { cursor: pointer; }
.viewer-root.editing .tile-surface { cursor: grab; }
.viewer-root.editing .tile-surface:active { cursor: grabbing; }

/* resize handles sitting on each split line */
.mosaic-gutter {
  position: absolute;
  z-index: 6;
  border: 0;
  padding: 0;
  background: transparent;
}
.mosaic-gutter::after {
  content: "";
  position: absolute;
  inset: 0;
  margin: auto;
  border-radius: 999px;
  background: rgba(181, 232, 83, .25);
  opacity: 0;
  transition: opacity .12s ease, background .12s ease;
}
.mosaic-gutter.row { width: 14px; transform: translateX(-50%); cursor: col-resize; }
.mosaic-gutter.row::after { width: 3px; height: 36px; top: 50%; transform: translateY(-50%); }
.mosaic-gutter.col { height: 14px; transform: translateY(-50%); cursor: row-resize; }
.mosaic-gutter.col::after { height: 3px; width: 36px; left: 50%; transform: translateX(-50%); }
.mosaic-gutter:hover::after, .mosaic-gutter:active::after { opacity: 1; background: var(--live); }

/* dock preview while dragging a camera onto another */
.mosaic-dock {
  position: absolute;
  z-index: 7;
  border-radius: var(--radius-tile);
  border: 1px solid rgba(181, 232, 83, .7);
  background: rgba(181, 232, 83, .16);
  pointer-events: none;
  transition: left .08s ease, top .08s ease, width .08s ease, height .08s ease;
}

/* edit-mode per-tile tools */
.tile-edit {
  position: absolute;
  inset: 8px 8px auto 8px;
  z-index: 4;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  pointer-events: none;
}
.tile-edit > * { pointer-events: auto; }
.tile-tag {
  pointer-events: none;
  background: rgba(7, 7, 9, .72);
  color: var(--ink-soft);
  border-radius: var(--radius-sm);
  padding: 5px 10px;
  font-size: 10px;
  letter-spacing: .12em;
  text-transform: uppercase;
  max-width: 60%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tile-edit-actions {
  display: flex;
  gap: 4px;
  background: rgba(7, 7, 9, .72);
  border-radius: var(--radius-sm);
  padding: 3px;
}

/* auto-hiding control cluster */
.viewer-hud {
  position: absolute;
  z-index: 9;
  left: 50%;
  bottom: 18px;
  transform: translateX(-50%);
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px;
  border-radius: 999px;
  background: rgba(12, 12, 14, .82);
  box-shadow: 0 12px 40px rgba(0, 0, 0, .5);
  backdrop-filter: blur(8px);
}
/* pill bar → pill-shaped buttons inside it */
.viewer-hud :deep(.btn) { border-radius: 999px; }
.audio-toggle svg {
  width: 15px;
  height: 15px;
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.viewer-error {
  position: absolute;
  z-index: 10;
  top: 16px;
  left: 50%;
  transform: translateX(-50%);
  max-width: min(560px, 90%);
  padding: 10px 16px;
  border-radius: var(--radius-sm);
  background: var(--danger-bg);
  color: var(--danger);
  font-size: 12px;
  cursor: pointer;
}

.hud-enter-active, .hud-leave-active { transition: opacity .18s ease, transform .18s ease; }
.hud-enter-from, .hud-leave-to { opacity: 0; transform: translate(-50%, 8px); }
</style>
