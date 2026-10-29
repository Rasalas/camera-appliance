<template>
  <div
    class="viewer-root"
    :class="rootClass"
    tabindex="0"
    aria-label="Live-Ansicht"
    @pointerdown.capture="revealControls"
    @focusin="revealControls"
    @keydown="revealControls"
    @pointermove="onRootPointerMove"
    @mouseleave="onRootMouseLeave"
    @selectstart.prevent
  >
    <div class="viewer-stage">
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
            <div class="viewer-frame-wrap" :ref="(el) => setFrameWrap(pane.alias, el)">
              <div
                v-if="planFor(pane.alias).sub"
                class="viewer-frame-transform"
                :class="displayClass(pane.slot)"
                :style="displayStyle(pane.slot)"
              >
                <iframe
                  class="viewer-frame"
                  :ref="(el) => setFrameRef(pane.alias, el)"
                  :src="frameSrc(pane.slot)"
                  :title="pane.slot.label"
                  allow="autoplay; fullscreen; picture-in-picture"
                  @load="syncAudioState"
                />
              </div>
              <div
                v-if="planFor(pane.alias).hd"
                class="viewer-frame-transform viewer-frame-transform-hd"
                :class="[displayClass(pane.slot), { ready: hdStatus[pane.alias] === 'playing' }]"
                :style="displayStyle(pane.slot)"
              >
                <iframe
                  class="viewer-frame"
                  :ref="(el) => setHDFrameRef(pane.alias, el)"
                  :src="hdFrameSrc(pane.slot)"
                  :title="`${pane.slot.label} HD`"
                  allow="autoplay; fullscreen; picture-in-picture"
                  @load="syncAudioState"
                />
              </div>
              <div
                v-if="placeholderKind(pane.slot) === 'offline'"
                class="viewer-placeholder offline"
                :title="canAdmin ? pane.slot.message : undefined"
              >
                <div class="placeholder-label">{{ pane.slot.label }}</div>
                <div class="placeholder-message">{{ offlineMessage(pane.slot.state) }}</div>
                <div v-if="canAdmin && !kioskMode && pane.slot.message" class="placeholder-detail">{{ pane.slot.message }}</div>
              </div>
              <div v-else-if="placeholderKind(pane.slot) === 'paused'" class="viewer-placeholder paused">
                <div class="placeholder-label">{{ pane.slot.label }}</div>
                <div class="placeholder-message">Pausiert – Leistungsmodus „Niedrig“</div>
              </div>
              <div v-if="isPausedByPerformance(pane.slot)" class="viewer-cover performance-cover">
                <span>Standby</span>
              </div>
              <div v-if="effectiveState(pane.slot) === 'connecting'" class="viewer-cover">
                <span class="loader-dot" />
                <span>{{ streamStatus(pane.alias) === 'stalled' ? 'Verbindet neu' : 'Verbindet' }}</span>
              </div>
            </div>

            <div
              class="tile-surface"
              role="button"
              :tabindex="spotlightAlias && spotlightAlias !== pane.alias ? -1 : 0"
              :aria-label="spotlightAlias === pane.alias ? `${pane.slot.label} verkleinern` : `${pane.slot.label} groß anzeigen`"
              @keydown.enter.prevent="onTileKey(pane.alias)"
              @keydown.space.prevent="onTileKey(pane.alias)"
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
                  :title="(effectiveDisplay(pane.slot).fit_mode === 'cover' ? 'Ganzes Bild zeigen' : 'Format füllen, Bildränder werden abgeschnitten') + (viewName ? ' · nur diese Ansicht' : ' · alle Geräte')"
                  :aria-label="`${pane.slot.label}: ${effectiveDisplay(pane.slot).fit_mode === 'cover' ? 'Füllen' : 'Ganzes Bild'}`"
                  :aria-pressed="effectiveDisplay(pane.slot).fit_mode === 'cover'"
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
            :aria-label="gutter.dir === 'row' ? 'Breite der Kameras ändern' : 'Höhe der Kameras ändern'"
            @pointerdown="startGutterDrag($event, gutter)"
          />
        </template>

        <div v-if="dockTarget" class="mosaic-dock" :style="dockOverlayStyle" aria-hidden="true" />

        <div v-if="!panes.length" class="viewer-empty">
          <div>Noch keine Kamera ausgewählt.</div>
          <RouterLink v-if="canAdmin" class="btn sm" to="/einrichtung">Kameras aktivieren</RouterLink>
        </div>
      </section>
    </div>

    <transition name="controls">
      <footer v-if="showHud" class="viewer-controls" aria-label="Kamerasteuerung" @pointermove.stop="revealControls" @click.stop>
        <div class="viewer-hud">
          <label class="view-picker">Ansicht
            <select :value="viewName" @change="switchView(($event.target as HTMLSelectElement).value)">
              <option value="">Standard</option>
              <option v-for="name in Object.keys(viewer?.profiles ?? {}).sort()" :key="name" :value="name">{{ name }}</option>
              <option v-if="viewName && !viewer?.profiles?.[viewName]" :value="viewName">{{ viewName }}</option>
            </select>
          </label>
          <template v-if="canAdmin">
            <button class="btn sm" :class="{ live: editing }" type="button" @click="toggleEdit">{{ editing ? 'Fertig' : 'Bearbeiten' }}</button>
          </template>
          <template v-if="editing && canAdmin">
            <span class="view-scope">{{ viewName ? 'Bildmodus je Kamera nur für diese Ansicht' : 'Bildmodus in Standard gilt für alle Geräte' }}</span>
            <button class="btn sm" type="button" @click="applyPortraitGrid">4 + Hochkant rechts</button>
            <input v-model="newViewName" class="view-name" aria-label="Name der neuen Ansicht" placeholder="z. B. ipad" maxlength="48" />
            <button class="btn sm" type="button" @click="createView">Als eigene Ansicht speichern</button>
          </template>
          <button class="btn sm" type="button" @click="showViewLink">Link zur Ansicht</button>
          <a v-if="viewLink" class="view-link" :href="viewLink">{{ viewLink }}</a>
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
      </footer>
    </transition>

    <transition name="hud">
      <div v-if="error" class="viewer-error" @click="error = ''">{{ error }}</div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { CSSProperties, Ref } from 'vue'
import { api } from '../api/client'
import type { AuthStatus, CameraDisplay, ViewerResponse, ViewerSlot, ViewerSlotState } from '../types'
import { ROOT_TARGET, layoutGeometry, treeSlots, setRatioAtPath, reconcileTree, dockCamera, parseMosaic, portraitGridTree } from './viewerMosaic'
import type { MosaicNode, PaneRect, GutterRect, Side } from './viewerMosaic'
import { STAGGER_MS, offlineMessage, parseEmbedMessage, planPlayer } from './viewerPlayback'
import type { FrameStatus, PlayerPlan } from './viewerPlayback'
import { browserStorage, createVersionWatch, resolveKioskFlag } from '../composables/viewerKiosk'
import { frameDisplayStyle } from './viewerDisplay'
import { createSnapshotSave } from '../composables/layoutSave'

// --- Mosaic layout model -----------------------------------------------------
// A binary split tree, like VSCode editor groups. A leaf shows one camera; a
// split divides its area into two children (row = side by side, col = stacked)
// with a ratio for the first child. Cameras dock onto a pane's edge to create a
// new split, or onto the centre to swap.

const viewName = ref(resolveViewName())
const newViewName = ref('')
const viewLink = ref('')
const frameSizes = ref<Record<string, { width: number; height: number }>>({})
const frameWraps = new Map<string, HTMLElement>()
let frameObserver: ResizeObserver | undefined
function setFrameWrap(alias: string, el: unknown) {
  const previous = frameWraps.get(alias)
  if (previous === el) return
  if (previous) frameObserver?.unobserve(previous)
  frameWraps.delete(alias)
  if (!(el instanceof HTMLElement)) return
  frameWraps.set(alias, el)
  frameObserver ??= new ResizeObserver((entries) => {
    const sizes = { ...frameSizes.value }
    for (const entry of entries) {
      const name = [...frameWraps].find(([, node]) => node === entry.target)?.[0]
      if (name) sizes[name] = { width: entry.contentRect.width, height: entry.contentRect.height }
    }
    frameSizes.value = sizes
  })
  frameObserver.observe(el)
}
function resolveViewName() {
  const param = new URLSearchParams(window.location.search).get('view')
  let stored = ''
  try { stored = browserStorage()?.getItem('camera-appliance.viewer.view') ?? '' } catch { /* Storage can be disabled. */ }
  const name = param ?? stored
  const valid = /^[a-z0-9][a-z0-9_-]{0,47}$/.test(name) ? name : ''
  if (param !== null) {
    try { browserStorage()?.setItem('camera-appliance.viewer.view', valid) } catch { /* URL still applies. */ }
  }
  return valid
}
const viewer = ref<ViewerResponse>()
const auth = ref<AuthStatus>()
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const subStatus = ref<Record<string, FrameStatus>>({})
const hdStatus = ref<Record<string, FrameStatus>>({})
const performanceMode = ref<'quality' | 'balanced' | 'low' | 'diagnostic'>('quality')
// Off until someone presses the sound control, so a single tap never starts audio.
const audioEnabled = ref(false)
const activeAudioAlias = ref('')

// Normal viewing reserves space for controls. Fullscreen and kiosk modes reveal
// them temporarily without resizing the camera surface.
const editing = ref(false)
const spotlightAlias = ref('')
const controlsVisible = ref(false)
const isFullscreen = ref(false)
const displayOverrides = ref<Record<string, CameraDisplay>>({})
const profileFits = ref<Record<string, CameraDisplay['fit_mode']>>({})

// Layout tree + live drag state.
const mosaic = ref<MosaicNode>()
const gridEl = ref<HTMLElement>()
const dragSourceAlias = ref('')
const dockTarget = ref('')
const dockSide = ref<Side>('center')

const pageHidden = ref(document.hidden)
const released = ref<ReadonlySet<string>>(new Set())

let refreshTimer = 0
let versionTimer = 0
let controlsTimer = 0
let tileClickTimer = 0
let displaySaveTimer = 0
let staggerTimer = 0
let spotlightIdleTimer = 0
let onAuthChanged: (() => void) | undefined
let onFullscreenChange: (() => void) | undefined
let onVisibilityChange: (() => void) | undefined
let onKey: ((e: KeyboardEvent) => void) | undefined
let onMessage: ((e: MessageEvent) => void) | undefined
let stopDrag: (() => void) | undefined
let stopCropPan: (() => void) | undefined
const frameEls: Record<string, HTMLIFrameElement> = {}
const hdFrameEls: Record<string, HTMLIFrameElement> = {}
const AUDIO_SETTING_KEY = 'camera-appliance.viewer.audioEnabled'
const LEGACY_AUDIO_SETTING_KEY = 'camera-appliance.viewer.audioHoverEnabled'
const SPOTLIGHT_IDLE_MS = 2 * 60 * 1000
const VERSION_CHECK_MS = 60 * 1000
const kioskMode = resolveKioskFlag(window.location.search, browserStorage())
const versionWatch = createVersionWatch()
const layoutSave = createSnapshotSave({
  delay: 400,
  save: async (json) => {
    try {
      await api.saveSettings({ [viewName.value ? `viewer.profiles.${viewName.value}.mosaic` : 'viewer.layout.mosaic']: json })
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Layout konnte nicht gespeichert werden.'
      throw err
    }
  }
})
const fitSave = createSnapshotSave({
  delay: 400,
  save: async (json) => {
    if (!viewName.value) return
    try {
      const fits = JSON.parse(json) as Record<string, CameraDisplay['fit_mode']>
      await api.saveSettings(Object.fromEntries(Object.entries(fits).map(([alias, fit]) => [`viewer.profiles.${viewName.value}.fit.${alias}`, fit])))
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Bildmodus konnte nicht gespeichert werden.'
      throw err
    }
  }
})
type ScreenWakeLock = { release: () => Promise<void>; released?: boolean }
let screenWakeLock: ScreenWakeLock | undefined

const slots = computed(() => viewer.value?.slots ?? [])
const slotByAlias = computed(() => new Map(slots.value.map((slot) => [slot.alias, slot])))
const canAdmin = computed(() => (auth.value ? !auth.value.enabled || auth.value.role === 'admin' : false))

const immersive = computed(() => isFullscreen.value || kioskMode)
// Hidden tabs stop streaming, except on the kiosk which must stay live.
const playersPaused = computed(() => pageHidden.value && !kioskMode)

const rootClass = computed(() => ({
  editing: editing.value,
  spotlight: !!spotlightAlias.value,
  fullscreen: isFullscreen.value,
  immersive: immersive.value,
  docking: !!dragSourceAlias.value,
  'cursor-hidden': immersive.value && !controlsVisible.value && !editing.value
}))
const showHud = computed(() => !immersive.value || controlsVisible.value || editing.value)

watch(immersive, () => revealControls())

// --- Tree geometry -----------------------------------------------------------


const geometry = computed(() => layoutGeometry(mosaic.value))

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

const plans = computed(() => {
  const ctx = {
    mode: performanceMode.value,
    primary: primaryLiveAlias.value,
    spotlight: spotlightAlias.value,
    released: released.value,
    paused: playersPaused.value,
    hdStatus: hdStatus.value
  }
  const result: Record<string, PlayerPlan> = {}
  for (const pane of panes.value) {
    const slot = pane.slot
    const hdURL = slot.playback?.hd_page_url
    result[pane.alias] = planPlayer({
      alias: pane.alias,
      playable: isPlayable(slot),
      hasHD: !!hdURL && hdURL !== slot.playback?.page_url
    }, ctx)
  }
  return result
})

function planFor(alias: string): PlayerPlan {
  return plans.value[alias] ?? { sub: false, hd: false, waiting: false }
}

// Balanced mode: secondary tiles join one at a time so a weak laptop does not
// open every stream at once.
const staggerQueue = computed(() => Object.entries(plans.value).filter(([, plan]) => plan.waiting).map(([alias]) => alias))

watch(staggerQueue, (queue) => {
  if (!queue.length || staggerTimer) return
  staggerTimer = window.setTimeout(() => {
    staggerTimer = 0
    const next = staggerQueue.value[0]
    if (next) released.value = new Set([...released.value, next])
  }, STAGGER_MS)
})

function restartStagger() {
  window.clearTimeout(staggerTimer)
  staggerTimer = 0
  released.value = new Set()
}

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

function onTileKey(alias: string) {
  if (editing.value) return
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
  if (mosaic.value) setTree(dockCamera(mosaic.value, sourceAlias, targetAlias, side))
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
  if (!canAdmin.value || !mosaic.value) return
  layoutSave.schedule(mosaic.value)
}

function applyPortraitGrid() {
  const aliases = treeSlots(mosaic.value)
  if (aliases.length !== 5) { error.value = 'Diese Vorlage benötigt fünf Kameras.'; return }
  const portrait = slots.value.find((slot) => aliases.includes(slot.alias) && [90, 270].includes(effectiveDisplay(slot).rotation))?.alias ?? aliases[4]
  setTree(portraitGridTree(aliases, portrait))
}

async function switchView(name: string) {
  try {
    await Promise.all([layoutSave.flush(), fitSave.flush()])
  } catch { return }
  try { browserStorage()?.setItem('camera-appliance.viewer.view', name) } catch { /* URL still works without storage. */ }
  const url = new URL(window.location.href)
  url.searchParams.set('view', name)
  window.location.assign(url.href)
}

async function createView() {
  const name = newViewName.value.trim().toLowerCase()
  if (!/^[a-z0-9][a-z0-9_-]{0,47}$/.test(name)) {
    error.value = 'Bitte einen Namen aus a–z, 0–9, Bindestrich oder Unterstrich eingeben.'
    return
  }
  if (viewer.value?.profiles?.[name] || name === viewName.value) {
    error.value = 'Diese Ansicht existiert bereits. Zum Bearbeiten bitte oben auswählen.'
    return
  }
  if (!mosaic.value) return
  try {
    await api.saveSettings({
      [`viewer.profiles.${name}.mosaic`]: JSON.stringify(mosaic.value),
      ...Object.fromEntries(slots.value.map((slot) => [`viewer.profiles.${name}.fit.${slot.alias}`, effectiveDisplay(slot).fit_mode]))
    })
    await switchView(name)
  } catch (err) { error.value = err instanceof Error ? err.message : 'Ansicht konnte nicht gespeichert werden.' }
}

function showViewLink() {
  const url = new URL(window.location.href)
  url.searchParams.set('view', viewName.value)
  viewLink.value = url.href
}

// --- Chrome: controls auto-hide, spotlight, edit, fullscreen -----------------

function revealControls() {
  armSpotlightIdle()
  controlsVisible.value = true
  window.clearTimeout(controlsTimer)
  if (!immersive.value || editing.value) return
  controlsTimer = window.setTimeout(() => {
    controlsVisible.value = false
  }, 2600)
}

function onRootPointerMove(event: PointerEvent) {
  if (event.pointerType === 'touch') return
  revealControls()
}

function scheduleHideControls() {
  if (!immersive.value || editing.value) return
  window.clearTimeout(controlsTimer)
  controlsTimer = window.setTimeout(() => {
    controlsVisible.value = false
  }, 600)
}

function onRootMouseLeave() {
  scheduleHideControls()
}

function toggleEdit() {
  const leaving = editing.value
  editing.value = !leaving
  spotlightAlias.value = ''
  // Save the layout as confirmed by "Fertig", before a refresh can replace it.
  if (leaving) void Promise.all([layoutSave.flush(), fitSave.flush()]).catch(() => undefined)
  revealControls()
}

function toggleSpotlight(alias: string) {
  spotlightAlias.value = spotlightAlias.value === alias ? '' : alias
}

// Kiosk: an accidental tap must not pin one camera forever.
function armSpotlightIdle() {
  window.clearTimeout(spotlightIdleTimer)
  if (!kioskMode || !spotlightAlias.value) return
  spotlightIdleTimer = window.setTimeout(() => { spotlightAlias.value = '' }, SPOTLIGHT_IDLE_MS)
}

watch(spotlightAlias, (next, previous) => {
  if (previous && !next) restartStagger()
  armSpotlightIdle()
})

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
  if (isFullscreen.value || kioskMode) void requestScreenWakeLock()
  else void releaseScreenWakeLock()
}

async function requestScreenWakeLock() {
  if (document.visibilityState !== 'visible' || screenWakeLock && !screenWakeLock.released) return
  const wakeLock = (navigator as Navigator & { wakeLock?: { request: (type: 'screen') => Promise<ScreenWakeLock> } }).wakeLock
  if (!wakeLock) return
  try {
    screenWakeLock = await wakeLock.request('screen')
  } catch {
    // Unsupported contexts (for example plain HTTP on a LAN IP) degrade gracefully.
  }
}

async function releaseScreenWakeLock() {
  const lock = screenWakeLock
  screenWakeLock = undefined
  if (lock && !lock.released) await lock.release().catch(() => undefined)
}

// --- Audio -------------------------------------------------------------------

function setFrameRef(alias: string, el: unknown) {
  trackFrame(frameEls, subStatus, alias, el)
}

function setHDFrameRef(alias: string, el: unknown) {
  trackFrame(hdFrameEls, hdStatus, alias, el)
}

// A new or removed iframe starts over as "connecting" until its embed reports playback.
function trackFrame(frames: Record<string, HTMLIFrameElement>, status: Ref<Record<string, FrameStatus>>, alias: string, el: unknown) {
  const next = el instanceof HTMLIFrameElement ? el : undefined
  if (frames[alias] === next) return
  if (next) frames[alias] = next
  else delete frames[alias]
  if (alias in status.value) {
    const { [alias]: _dropped, ...rest } = status.value
    status.value = rest
  }
  window.setTimeout(() => syncAudioState(), 0)
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

// Exactly one frame per camera carries audio: HD once it plays, otherwise the sub stream.
function syncAudioState() {
  const audible = (alias: string) => audioEnabled.value && alias === activeAudioAlias.value
  const hdPlaying = (alias: string) => !!hdFrameEls[alias] && hdStatus.value[alias] === 'playing'
  for (const [alias, frame] of Object.entries(frameEls)) {
    frame.contentWindow?.postMessage({ type: 'camera-audio', muted: !audible(alias) || hdPlaying(alias) }, window.location.origin)
  }
  for (const [alias, frame] of Object.entries(hdFrameEls)) {
    frame.contentWindow?.postMessage({ type: 'camera-audio', muted: !audible(alias) || !hdPlaying(alias) }, window.location.origin)
  }
}

// --- Camera display (transform + inline crop) --------------------------------

function effectiveDisplay(slot: ViewerSlot): CameraDisplay {
  const display = displayOverrides.value[slot.alias] ?? normalizedDisplay(slot.display)
  return viewName.value ? { ...display, fit_mode: profileFits.value[slot.alias] ?? display.fit_mode } : display
}

function displayClass(slot: ViewerSlot) {
  const display = effectiveDisplay(slot)
  return {
    'fit-contain': display.fit_mode === 'contain',
    'rotated-quarter': display.rotation === 90 || display.rotation === 270
  }
}

function displayStyle(slot: ViewerSlot) {
  return frameDisplayStyle(effectiveDisplay(slot), frameSizes.value[slot.alias])
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
  if (viewName.value && display.fit_mode !== effectiveDisplay(slot).fit_mode) setProfileFit(slot.alias, display.fit_mode)
  displayOverrides.value = { ...displayOverrides.value, [slot.alias]: display }
}

function rotateTile(slot: ViewerSlot) {
  const display = effectiveDisplay(slot)
  const rotation = (((display.rotation || 0) + 90) % 360) as CameraDisplay['rotation']
  setDisplayOverride(slot, { ...display, rotation })
  scheduleDisplaySave(slot)
}

function setProfileFit(alias: string, fit: CameraDisplay['fit_mode']) {
  profileFits.value = { ...profileFits.value, [alias]: fit }
  if (canAdmin.value) fitSave.schedule(profileFits.value)
}

function toggleFitTile(slot: ViewerSlot) {
  const display = effectiveDisplay(slot)
  const fit_mode = display.fit_mode === 'cover' ? 'contain' : 'cover'
  if (viewName.value) { setProfileFit(slot.alias, fit_mode); return }
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
      ...(!viewName.value ? { [`camera.display.${deviceID}.fit_mode`]: display.fit_mode } : {}),
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

// Playback presence decides, not state: a go2rtc outage only changes the status while players retry.
function isPlayable(slot: ViewerSlot) {
  return !!slot.playback?.page_url
}

function isPausedByPerformance(slot: ViewerSlot) {
  return performanceMode.value === 'low' && isPlayable(slot) && slot.alias !== primaryLiveAlias.value
}

function appendQuery(url: string, params: Record<string, string>) {
  const search = new URLSearchParams(params)
  const separator = url.includes('?') ? '&' : '?'
  return `${url}${separator}${search.toString()}`
}

function frameSrc(slot: ViewerSlot) {
  const url = slot.playback?.page_url
  if (!url) return ''
  return appendQuery(url, {
    fit: effectiveDisplay(slot).fit_mode,
    label: slot.label,
  })
}

function hdFrameSrc(slot: ViewerSlot) {
  const url = slot.playback?.hd_page_url
  if (!url) return ''
  return appendQuery(url, {
    fit: effectiveDisplay(slot).fit_mode,
    label: `${slot.label} HD`,
  })
}

// Spotlight-hidden, staggered and paused tiles show no placeholder text.
function placeholderKind(slot: ViewerSlot): 'offline' | 'paused' | '' {
  const plan = planFor(slot.alias)
  if (plan.sub || plan.hd) return ''
  if (isPausedByPerformance(slot)) return 'paused'
  if (!isPlayable(slot)) return 'offline'
  return ''
}

// What the viewer sees: HD once it plays, otherwise the sub stream.
function streamStatus(alias: string): FrameStatus | undefined {
  const plan = planFor(alias)
  if (plan.hd && hdStatus.value[alias] === 'playing') return 'playing'
  if (plan.sub) return subStatus.value[alias] ?? 'connecting'
  if (plan.hd) return hdStatus.value[alias] ?? 'connecting'
  return undefined
}

function effectiveState(slot: ViewerSlot): ViewerSlotState {
  if (!isPlayable(slot)) return slot.state
  if (planFor(slot.alias).waiting) return 'connecting'
  const status = streamStatus(slot.alias)
  // On "error" the embed shows its own explanation, so no overlay on top.
  if (status === 'connecting' || status === 'stalled') return 'connecting'
  return slot.state
}

function handleEmbedMessage(event: MessageEvent) {
  if (event.origin !== window.location.origin) return
  const message = parseEmbedMessage(event.data)
  if (!message) return
  const frame = (message.hd ? hdFrameEls : frameEls)[message.alias]
  // Ignore late messages from iframes that were already replaced.
  if (!frame || event.source !== frame.contentWindow) return
  const status = message.hd ? hdStatus : subStatus
  if (status.value[message.alias] === message.state) return
  status.value = { ...status.value, [message.alias]: message.state }
  syncAudioState()
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
    if (!editing.value && !fitSave.busy()) profileFits.value = viewerData.profile_fits?.[viewName.value] ?? {}
    performanceMode.value = normalizedPerformanceMode(viewerData.performance?.mode)
    // Only cameras that are activated (have a binding) appear in the viewer; their
    // placement is controlled here, slots are assigned automatically in the background.
    const aliases = viewerData.slots.filter((slot) => slot.binding?.device_id).map((slot) => slot.alias)
    // Keep the local tree while it is being edited or still being saved.
    if (!editing.value && !layoutSave.busy()) {
      const raw = viewName.value ? viewerData.profiles?.[viewName.value] : viewerData.layout?.mosaic
      const portrait = viewerData.slots.find((slot) => aliases.includes(slot.alias) && [90, 270].includes(effectiveDisplay(slot).rotation))?.alias
      const initial = parseMosaic(raw) ?? (portrait ? portraitGridTree(aliases, portrait) : undefined)
      mosaic.value = aliases.length ? reconcileTree(initial, aliases) : undefined
    }
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

// Kiosks without an admin session would otherwise keep the old frontend after an update.
async function checkVersion() {
  try {
    const health = await api.health()
    if (versionWatch.changed(`${health.version}@${health.commit}`) && !editing.value) window.location.reload()
  } catch {
    // Unreachable during restarts; the next check catches up.
  }
}

function readAudioSetting() {
  try {
    return (window.localStorage.getItem(AUDIO_SETTING_KEY) ?? window.localStorage.getItem(LEGACY_AUDIO_SETTING_KEY)) === 'true'
  } catch {
    return false
  }
}

onMounted(() => {
  audioEnabled.value = readAudioSetting()
  void refreshAuth()
  onAuthChanged = () => void refreshAuth()
  window.addEventListener('auth-changed', onAuthChanged)
  onFullscreenChange = () => syncFullscreen()
  document.addEventListener('fullscreenchange', onFullscreenChange)
  onVisibilityChange = () => {
    pageHidden.value = document.hidden
    if (document.hidden) return
    if (isFullscreen.value || kioskMode) void requestScreenWakeLock()
    if (!kioskMode) {
      restartStagger()
      void load()
      void checkVersion()
    }
  }
  document.addEventListener('visibilitychange', onVisibilityChange)
  if (kioskMode) void requestScreenWakeLock()
  onMessage = (event) => handleEmbedMessage(event)
  window.addEventListener('message', onMessage)
  onKey = (event: KeyboardEvent) => {
    if (event.key !== 'Escape') return
    if (spotlightAlias.value) spotlightAlias.value = ''
    else if (editing.value) toggleEdit()
  }
  window.addEventListener('keydown', onKey)
  void load()
  void checkVersion()
  refreshTimer = window.setInterval(() => {
    if (!busy.value && !playersPaused.value) void load()
  }, 15000)
  versionTimer = window.setInterval(() => {
    if (!playersPaused.value) void checkVersion()
  }, VERSION_CHECK_MS)
})

onBeforeUnmount(() => {
  frameObserver?.disconnect()
  muteAllAudio()
  stopDrag?.()
  stopCropPan?.()
  window.clearInterval(refreshTimer)
  window.clearInterval(versionTimer)
  window.clearTimeout(controlsTimer)
  window.clearTimeout(tileClickTimer)
  window.clearTimeout(displaySaveTimer)
  window.clearTimeout(staggerTimer)
  window.clearTimeout(spotlightIdleTimer)
  void Promise.all([layoutSave.flush(), fitSave.flush()]).catch(() => undefined)
  if (onAuthChanged) window.removeEventListener('auth-changed', onAuthChanged)
  if (onFullscreenChange) document.removeEventListener('fullscreenchange', onFullscreenChange)
  if (onVisibilityChange) document.removeEventListener('visibilitychange', onVisibilityChange)
  void releaseScreenWakeLock()
  if (onMessage) window.removeEventListener('message', onMessage)
  if (onKey) window.removeEventListener('keydown', onKey)
})
</script>

<style scoped>
.view-scope { flex-basis: 100%; text-align: center; font-size: 12px; color: var(--ink-mute); }
.view-picker { display: flex; align-items: center; gap: 8px; font-size: 13px; }
.view-picker select, .view-name { min-height: 44px; max-width: 170px; background: var(--bg); color: var(--ink); border: 1px solid var(--hairline-strong); border-radius: 8px; padding: 8px; }
.view-link { flex-basis: 100%; overflow-wrap: anywhere; text-align: center; font-size: 12px; }
.tile-surface, .mosaic-gutter { touch-action: none; }

/* The camera stage scrolls independently above the intrinsic-height footer. */
.viewer-root {
  position: relative;
  display: grid;
  grid-template-rows: minmax(0, 1fr) auto;
  height: 100vh;
  height: 100dvh;
  background: var(--bg);
  overflow: hidden;
  user-select: none;
  -webkit-user-select: none;
}

.viewer-stage {
  position: relative;
  min-height: 0;
  overflow: hidden;
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
.tile-surface:focus-visible {
  outline: 2px solid var(--live);
  outline-offset: -2px;
  border-radius: var(--radius-tile);
}
.viewer-root.cursor-hidden,
.viewer-root.cursor-hidden * { cursor: none !important; }
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

/* The footer owns its space; only immersive controls overlay the cameras. */
.viewer-controls {
  z-index: 9;
  display: flex;
  justify-content: center;
  padding: 6px max(12px, env(safe-area-inset-right)) max(10px, env(safe-area-inset-bottom)) max(12px, env(safe-area-inset-left));
  background: var(--bg);
}
.viewer-root.immersive .viewer-controls {
  position: absolute;
  inset: auto 0 0;
  padding-bottom: max(18px, env(safe-area-inset-bottom));
  background: transparent;
}
.viewer-hud {
  width: max-content;
  max-width: 100%;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px;
  border-radius: 24px;
  background: rgba(12, 12, 14, .82);
  backdrop-filter: blur(8px);
}
.viewer-root.immersive .viewer-hud {
  box-shadow: 0 12px 40px rgba(0, 0, 0, .5);
}
.controls-enter-active, .controls-leave-active { transition: opacity .18s ease, transform .18s ease; }
.controls-enter-from, .controls-leave-to { opacity: 0; transform: translateY(8px); }
/* pill bar → pill-shaped buttons inside it */
.viewer-hud :deep(.btn) { border-radius: 999px; }
/* touch targets of at least 44px with readable labels */
.viewer-hud :deep(.btn.sm),
.tile-edit-actions .btn.sm {
  min-height: 44px;
  padding: 10px 18px;
  font-size: 13px;
}
.viewer-hud :deep(.btn.icon.sm),
.tile-edit-actions .btn.icon.sm {
  width: 44px;
  min-width: 44px;
  padding: 0;
  font-size: 20px;
}
.audio-toggle svg {
  width: 20px;
  height: 20px;
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
