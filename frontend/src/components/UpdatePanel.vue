<template>
  <div v-if="visible" class="update-rail">
    <button
      ref="triggerEl"
      class="rail-update"
      :class="{ attention: hasUpdate, ready: isReady, busy: isWorking }"
      type="button"
      :aria-label="triggerLabel"
      :title="triggerLabel"
      aria-haspopup="dialog"
      :aria-expanded="hoverOpen || mobileOpen ? 'true' : 'false'"
      @click="onTriggerClick"
      @pointerenter="onTriggerEnter"
      @pointerleave="onTriggerLeave"
      @focus="onTriggerFocus"
      @blur="onTriggerBlur"
    >
      <!-- Download arrow for a pending release, install arrow when ready,
           refresh otherwise. -->
      <svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <template v-if="isReady">
          <path d="M12 21V9" /><path d="m7 14 5-5 5 5" /><path d="M4 3h16" />
        </template>
        <template v-else-if="hasUpdate || phase === 'downloading'">
          <path d="M12 3v12" /><path d="m7 10 5 5 5-5" /><path d="M4 21h16" />
        </template>
        <template v-else>
          <path d="M21 12a9 9 0 1 1-2.64-6.36" /><path d="M21 3v6h-6" />
        </template>
      </svg>
      <span v-if="showBadge" class="update-badge" />

      <!-- Progress ring: fills with the real download share, spins while the
           size is unknown or an install runs. -->
      <svg v-if="ringVisible" class="update-ring" viewBox="0 0 34 34" aria-hidden="true">
        <circle class="ring-track" cx="17" cy="17" r="15" />
        <circle
          v-if="ringDeterminate"
          class="ring-fill"
          cx="17"
          cy="17"
          r="15"
          :style="ringStyle"
        />
        <circle v-else class="ring-fill ring-indet" cx="17" cy="17" r="15" />
      </svg>
    </button>

    <!-- Desktop: pure info sheet on hover — deliberately no controls inside,
         the trigger button itself drives the flow. -->
    <Teleport to="body">
      <transition name="update-pop">
        <div
          v-if="hoverOpen"
          ref="popEl"
          class="update-pop"
          :class="placement"
          :style="popStyle"
          role="dialog"
          aria-modal="false"
          :aria-label="headline"
          @pointerenter="cancelClose"
          @pointerleave="scheduleClose"
        >
          <header class="update-head">
            <h3>{{ headline }}</h3>
            <span v-if="versionChip" class="update-version">{{ versionChip }}</span>
          </header>
          <div class="update-body">
            <UpdateInfo :phase="phase" :status="status" />
          </div>
        </div>
      </transition>
    </Teleport>

    <!-- Touch devices: tap opens a bottom sheet with a confirm button; it
         can be pulled down and flicked away by its grab area. -->
    <Teleport to="body">
      <transition name="update-sheet">
        <div v-if="mobileOpen" class="sheet-backdrop" @click.self="closeMobile">
          <div ref="sheetEl" class="update-sheet" :class="{ dragging }" :style="sheetStyle" role="dialog" aria-modal="true" :aria-label="headline" @pointerdown="onDragStart" @pointermove="onDragMove" @pointerup="onDragEnd" @pointercancel="onDragEnd">
            <span class="sheet-grab" aria-hidden="true" />
            <header class="update-head sheet-head">
              <h3>{{ headline }}</h3>
              <span v-if="versionChip" class="update-version">{{ versionChip }}</span>
              <button class="update-icon-btn" type="button" aria-label="Schließen" title="Schließen" @click="closeMobile">
                <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" aria-hidden="true">
                  <path d="M6 6l12 12M18 6 6 18" />
                </svg>
              </button>
            </header>

            <div ref="sheetBodyEl" class="update-body sheet-body" :class="{ 'body-at-top': bodyAtTop }" @scroll.passive="onBodyScroll">
              <UpdateInfo :phase="phase" :status="status" />
            </div>

            <footer v-if="action" class="sheet-foot">
              <button
                class="btn sm update-action"
                :class="action.variant"
                type="button"
                :disabled="busy || isWorking"
                @click="action.run"
              >
                {{ action.label }}
              </button>
            </footer>
          </div>
        </div>
      </transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api } from '../api/client'
import { createUpdateClient } from '../composables/updateClient'
import type { UpdateFlowPhase, UpdateFlowStatus } from '../types'
import UpdateInfo from './UpdateInfo.vue'

const props = defineProps<{ visible: boolean }>()

const status = ref<UpdateFlowStatus>()
const busy = ref(false)
const triggerEl = ref<HTMLButtonElement>()
const popEl = ref<HTMLElement>()
const sheetEl = ref<HTMLElement>()
const hoverOpen = ref(false)
const mobileOpen = ref(false)
let started = false
let pollTimer = 0
let autoCheckTimer = 0
let enterTimer = 0
let closeTimer = 0
let dragResetTimer = 0
let prevBodyOverflow = ''

/* Drag-to-dismiss state for the bottom sheet. The whole sheet is grabbable;
   inside the scrollable changelog a downward drag only dismisses while the
   content sits at its very top, otherwise it scrolls the content. */
const dragging = ref(false)
const dragY = ref(0)
const sheetBodyEl = ref<HTMLElement>()
const bodyAtTop = ref(true)
type DragPhase = 'idle' | 'pending' | 'active'
let dragPhase: DragPhase = 'idle'
let dragStartY = 0
let dragStartTime = 0
let dragOrigin: EventTarget | null = null

const sheetStyle = computed(() => ({ transform: `translateY(${dragY.value}px)` }))

function onBodyScroll() {
  bodyAtTop.value = (sheetBodyEl.value?.scrollTop ?? 0) <= 0
}

function onDragStart(event: PointerEvent) {
  // Buttons and release links must stay plain clicks, not drag starts.
  if ((event.target as HTMLElement | null)?.closest('button, a')) return
  if (event.pointerType === 'mouse' && event.button !== 0) return
  dragPhase = 'pending'
  dragOrigin = event.target
  dragStartY = event.clientY
  dragStartTime = performance.now()
  try {
    ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
  } catch {
    /* synthetic or already-released pointer */
  }
}

// Decide on first significant motion whether this gesture moves the sheet.
function dragTakesOver(deltaY: number): boolean {
  const body = sheetBodyEl.value
  if (dragOrigin && body?.contains(dragOrigin as Node)) {
    if (!bodyAtTop.value) return false
    return deltaY > 0
  }
  return true
}

function onDragMove(event: PointerEvent) {
  if (dragPhase === 'idle') return
  const deltaY = event.clientY - dragStartY
  if (dragPhase === 'pending') {
    if (Math.abs(deltaY) < 6) return
    dragPhase = dragTakesOver(deltaY) ? 'active' : 'idle'
    if (dragPhase !== 'active') return
    dragging.value = true
  }
  // Only downward motion counts; upward drags resist instead of following.
  dragY.value = Math.max(0, deltaY)
}

function onDragEnd(event: PointerEvent) {
  const wasActive = dragPhase === 'active'
  dragPhase = 'idle'
  dragging.value = false
  if (!wasActive) return
  const delta = Math.max(0, event.clientY - dragStartY)
  const elapsed = performance.now() - dragStartTime
  const velocity = elapsed > 0 ? delta / elapsed : 0
  const height = sheetEl.value?.offsetHeight ?? 400
  const farEnough = delta > Math.max(80, height * 0.22)
  const flicked = velocity > 0.6 && delta > 24
  if (farEnough || flicked) {
    // Slide fully off-screen while the backdrop fades, then unmount.
    dragY.value = window.innerHeight
    closeMobile()
  } else {
    dragY.value = 0
  }
}

const AUTO_CHECK_INTERVAL = 6 * 60 * 60 * 1000
const POP_WIDTH = 340
const POP_GAP = 10
const VIEWPORT_MARGIN = 12
const MIN_POP_HEIGHT = 180
// Keep the panel a popover, not a full-height sheet, even with a long changelog.
const MAX_POP_HEIGHT = 460

/* Hover vs touch decides how the sheet opens: hover shows an info-only
   popover and clicks drive the flow directly; coarse pointers get a bottom
   sheet whose button confirms each step. */
const touchQuery = window.matchMedia('(hover: none), (pointer: coarse)')
const isTouch = ref(touchQuery.matches)
function onTouchModeChange(event: MediaQueryListEvent) {
  isTouch.value = event.matches
  hoverOpen.value = false
  mobileOpen.value = false
}

function normalizeVersion(value: string | undefined): string {
  return (value ?? '').trim().replace(/^v/i, '')
}

// The backend already filters out releases that are not newer, but the whole UI
// must never advertise an update for the version that is already installed.
const hasNewerRelease = computed(() => {
  const tag = normalizeVersion(status.value?.latest?.tag)
  return tag !== '' && tag !== normalizeVersion(status.value?.current_version)
})

// Single source of truth: heading, body, icon, badge and click flow read this.
const phase = computed<UpdateFlowPhase>(() => {
  const raw = status.value?.phase ?? 'idle'
  if (raw === 'available' && !hasNewerRelease.value) return 'up_to_date'
  return raw
})

const hasUpdate = computed(() => phase.value === 'available')
const isReady = computed(() => phase.value === 'ready')
const isWorking = computed(() => phase.value === 'downloading' || phase.value === 'installing')
const showBadge = computed(() => hasUpdate.value || isReady.value)

/* ---------- Progress ring ------------------------------------------------ */

const RING_R = 15
const RING_CIRC = (2 * Math.PI * RING_R).toFixed(2)
const ringVisible = computed(() => phase.value === 'downloading' || phase.value === 'installing')
const ringDeterminate = computed(() => phase.value === 'downloading' && (status.value?.total ?? 0) > 0)

const progressValue = computed(() => {
  const total = status.value?.total ?? 0
  if (total <= 0) return 0
  return Math.min(1, Math.max(0, (status.value?.downloaded ?? 0) / total))
})

const ringPercent = computed(() => Math.floor(progressValue.value * 100))

const ringStyle = computed(() => ({
  strokeDasharray: RING_CIRC,
  strokeDashoffset: (2 * Math.PI * RING_R * (1 - progressValue.value)).toFixed(2)
}))

const headline = computed(() => {
  switch (phase.value) {
    case 'available':
      return 'Update verfügbar'
    case 'downloading':
      return 'Update wird geladen'
    case 'ready':
      return 'Update bereit'
    case 'installing':
      return 'Installation läuft'
    case 'failed':
      return 'Update fehlgeschlagen'
    case 'checking':
      return 'Suche nach Updates'
    case 'up_to_date':
      return 'Alles aktuell'
    default:
      return 'Updates'
  }
})

const triggerLabel = computed(() => {
  switch (phase.value) {
    case 'available':
      return 'Update herunterladen'
    case 'downloading':
      return ringDeterminate.value ? `Update wird geladen (${ringPercent.value}%)` : 'Update wird geladen'
    case 'ready':
      return 'Update jetzt installieren'
    case 'installing':
      return 'Installation läuft'
    case 'checking':
      return 'Suche nach Updates'
    case 'failed':
      return 'Update fehlgeschlagen – erneut suchen'
    default:
      return 'Nach Updates suchen'
  }
})

const versionChip = computed(() => {
  if (phase.value === 'available' || phase.value === 'ready') return status.value?.latest?.tag ?? ''
  return ''
})

// The single next step for the current phase, offered in the mobile sheet.
const action = computed<{ label: string; variant: string; run: () => void } | null>(() => {
  switch (phase.value) {
    case 'available':
      return { label: 'Herunterladen', variant: 'primary', run: () => void download() }
    case 'ready':
      return { label: 'Jetzt aktualisieren', variant: 'primary', run: () => void install() }
    case 'failed':
      return { label: 'Erneut versuchen', variant: 'ghost', run: () => void check() }
    case 'idle':
      return { label: 'Nach Updates suchen', variant: 'ghost', run: () => void check() }
    default:
      return null
  }
})

/* ---------- Trigger interaction ------------------------------------------ */

// Desktop click flow: available → download, ready → install, otherwise a
// manual re-check. A running download or install cannot be clicked away.
function onTriggerClick() {
  if (isTouch.value) {
    mobileOpen.value = !mobileOpen.value
    if (mobileOpen.value) void refresh()
    return
  }
  switch (phase.value) {
    case 'available':
      void download()
      break
    case 'ready':
      void install()
      break
    case 'downloading':
    case 'installing':
    case 'checking':
      break
    default:
      void check()
  }
}

function onTriggerEnter() {
  if (isTouch.value) return
  window.clearTimeout(closeTimer)
  window.clearTimeout(enterTimer)
  // Small delay so sweeping across the rail does not flash the sheet.
  enterTimer = window.setTimeout(() => {
    hoverOpen.value = true
  }, 120)
}

function onTriggerLeave() {
  window.clearTimeout(enterTimer)
  scheduleClose()
}

function onTriggerFocus() {
  if (isTouch.value) return
  window.clearTimeout(closeTimer)
  hoverOpen.value = true
}

function onTriggerBlur(event: FocusEvent) {
  const next = event.relatedTarget as Node | null
  if (next && popEl.value?.contains(next)) return
  hoverOpen.value = false
}

function scheduleClose() {
  window.clearTimeout(closeTimer)
  // Grace period so the pointer can travel into the sheet without closing it.
  closeTimer = window.setTimeout(() => {
    hoverOpen.value = false
  }, 240)
}

function cancelClose() {
  window.clearTimeout(closeTimer)
}

function closeMobile() {
  mobileOpen.value = false
}

/* ---------- Popover placement --------------------------------------------
   The rail is a narrow column, so the popover is teleported to the body and
   positioned from the trigger rect: it right-aligns to the button, opens
   upward whenever there is room and is always clamped into the viewport. */

const placement = ref<'up' | 'down'>('up')
const popStyle = ref<Record<string, string>>({})

function positionPop() {
  const trigger = triggerEl.value
  if (!trigger) return
  const rect = trigger.getBoundingClientRect()
  const vw = window.innerWidth
  const vh = window.innerHeight
  const width = Math.min(POP_WIDTH, vw - VIEWPORT_MARGIN * 2)
  const spaceAbove = rect.top - VIEWPORT_MARGIN - POP_GAP
  const spaceBelow = vh - rect.bottom - VIEWPORT_MARGIN - POP_GAP
  const up = spaceAbove >= MIN_POP_HEIGHT || spaceAbove >= spaceBelow

  // Align the popover's right edge with the trigger, then clamp into view.
  let left = rect.right - width
  left = Math.min(left, vw - VIEWPORT_MARGIN - width)
  left = Math.max(VIEWPORT_MARGIN, left)

  const room = up ? spaceAbove : spaceBelow
  const maxHeight = Math.max(MIN_POP_HEIGHT, Math.floor(Math.min(room, MAX_POP_HEIGHT, vh * 0.62)))

  const style: Record<string, string> = {
    left: `${Math.round(left)}px`,
    width: `${Math.round(width)}px`,
    maxHeight: `${maxHeight}px`
  }
  if (up) {
    style.bottom = `${Math.round(vh - rect.top + POP_GAP)}px`
  } else {
    style.top = `${Math.round(rect.bottom + POP_GAP)}px`
  }
  placement.value = up ? 'up' : 'down'
  popStyle.value = style
}

function bindPopListeners() {
  window.addEventListener('resize', positionPop)
  window.addEventListener('scroll', positionPop, true)
}

function unbindPopListeners() {
  window.removeEventListener('resize', positionPop)
  window.removeEventListener('scroll', positionPop, true)
}

/* ---------- API flow ------------------------------------------------------ */

const updateClient = createUpdateClient(api, {
  publish: value => { status.value = value },
  busy: value => { busy.value = value },
  reload: () => window.location.reload(),
  schedule: callback => { pollTimer = window.setTimeout(callback, 1000) },
  cancel: () => window.clearTimeout(pollTimer)
})
const { refresh, check, download, install } = updateClient

watch(hoverOpen, (isOpen) => {
  if (!isOpen) {
    unbindPopListeners()
    return
  }
  positionPop()
  bindPopListeners()
  void nextTick(positionPop)
})

watch(mobileOpen, (isOpen) => {
  if (isOpen) {
    window.clearTimeout(dragResetTimer)
    dragY.value = 0
    bodyAtTop.value = true
    prevBodyOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
  } else {
    document.body.style.overflow = prevBodyOverflow
    // Reset the drag offset only after the leave transition has unmounted
    // the sheet, so a flicked-away sheet never snaps back visibly.
    dragResetTimer = window.setTimeout(() => {
      dragY.value = 0
    }, 280)
  }
})

// The changelog changes the popover height, so re-anchor when the phase flips.
watch(phase, () => {
  if (hoverOpen.value) void nextTick(positionPop)
})

function onGlobalKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  hoverOpen.value = false
  mobileOpen.value = false
}

async function ensureStarted() {
  if (started || !props.visible) return
  started = true
  await refresh()
  // Auto-check on load when we have nothing cached yet.
  if (status.value?.phase === 'idle') void check()
  autoCheckTimer = window.setInterval(() => void check(), AUTO_CHECK_INTERVAL)
}

// Visibility depends on the async auth status, so start when it flips true.
watch(() => props.visible, (visible) => {
  if (visible) void ensureStarted()
  else {
    hoverOpen.value = false
    closeMobile()
  }
}, { immediate: true })

onMounted(() => {
  touchQuery.addEventListener('change', onTouchModeChange)
  document.addEventListener('keydown', onGlobalKeydown)
})

onBeforeUnmount(() => {
  updateClient.close()
  window.clearInterval(autoCheckTimer)
  window.clearTimeout(enterTimer)
  window.clearTimeout(closeTimer)
  window.clearTimeout(dragResetTimer)
  unbindPopListeners()
  touchQuery.removeEventListener('change', onTouchModeChange)
  document.removeEventListener('keydown', onGlobalKeydown)
  document.body.style.overflow = prevBodyOverflow
})
</script>

<style scoped>
.update-rail {
  display: grid;
  justify-items: end;
}

/* Collapsed control: quiet until something is actually pending. */
.rail-update {
  position: relative;
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: 999px;
  border: 1px solid var(--hairline);
  background: var(--raised);
  color: var(--ink-dim);
  cursor: pointer;
  transition: color .12s ease, border-color .12s ease, background .12s ease;
}
.rail-update:hover { color: var(--ink-soft); border-color: var(--hairline-strong); }
.rail-update:focus-visible {
  outline: 2px solid var(--live);
  outline-offset: 2px;
}
.rail-update.attention,
.rail-update.ready {
  color: var(--live);
  border-color: rgba(181, 232, 83, 0.42);
  background: var(--live-bg);
}
.rail-update.busy { color: var(--ink-soft); }
.update-badge {
  position: absolute;
  top: -2px;
  right: -2px;
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: var(--live);
  box-shadow: 0 0 0 2px var(--surface), 0 0 10px rgba(181, 232, 83, 0.7);
}

/* Progress ring drawn around the collapsed control. */
.update-ring {
  position: absolute;
  inset: -2px;
  width: calc(100% + 4px);
  height: calc(100% + 4px);
  transform: rotate(-90deg);
  pointer-events: none;
}
.ring-track {
  fill: none;
  stroke: var(--hairline);
  stroke-width: 2.4;
}
.ring-fill {
  fill: none;
  stroke: var(--live);
  stroke-width: 2.4;
  stroke-linecap: round;
  transition: stroke-dashoffset .4s linear;
}
.ring-indet {
  stroke-dasharray: 30 64.25;
  animation: ring-spin 1s linear infinite;
  transform-origin: 50% 50%;
}
@keyframes ring-spin { to { transform: rotate(360deg); } }

.update-pop {
  position: fixed;
  z-index: 200;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--hairline-strong);
  border-radius: var(--radius);
  background: var(--surface);
  box-shadow: 0 20px 56px rgba(0, 0, 0, .58);
}
.update-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 10px 8px 14px;
  border-bottom: 1px solid var(--hairline);
}
.update-head h3 {
  margin: 0;
  font-size: 12px;
  font-weight: 500;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--ink);
  overflow-wrap: anywhere;
}
.update-version {
  font-size: 10.5px;
  color: var(--live);
  letter-spacing: 0.04em;
}
.update-icon-btn {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  flex: 0 0 auto;
  margin-left: auto;
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--ink-dim);
  cursor: pointer;
}
.update-icon-btn:hover:not(:disabled) { color: var(--ink); background: var(--raised); }
.update-icon-btn:focus-visible {
  outline: 2px solid var(--live);
  outline-offset: 1px;
}
.update-body {
  padding: 12px 14px;
  display: grid;
  gap: 10px;
  align-content: start;
  overflow: hidden auto;
  overscroll-behavior: contain;
}

/* ---------- Mobile bottom sheet ---------------------------------------- */

.sheet-backdrop {
  position: fixed;
  inset: 0;
  z-index: 70;
  background: rgba(0, 0, 0, .62);
}
.update-sheet {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  max-height: min(78vh, 560px);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--hairline-strong);
  border-bottom: 0;
  border-radius: 18px 18px 0 0;
  background: var(--surface);
  box-shadow: 0 -20px 56px rgba(0, 0, 0, .58);
  padding-bottom: env(safe-area-inset-bottom);
  animation: sheet-up .22s cubic-bezier(.2, .8, .2, 1) both;
  will-change: transform;
}
/* Follow the finger while dragging; spring back or fly out otherwise. */
.update-sheet:not(.dragging) { transition: transform .24s cubic-bezier(.2, .8, .2, 1); }

/* Grab zones outside the scrollable content: the browser never turns these
   gestures into scrolls, so the sheet follows the finger one-to-one. */
.sheet-grab,
.sheet-head,
.sheet-foot { touch-action: none; }
.sheet-grab {
  display: block;
  width: 56px;
  padding: 10px 0 6px;
  margin: 0 auto;
  cursor: grab;
}
.sheet-grab::before {
  content: "";
  display: block;
  width: 36px;
  height: 4px;
  margin: 0 auto;
  border-radius: 999px;
  background: var(--hairline-strong);
}
.sheet-grab:active { cursor: grabbing; }
/* Inside the changelog the content scrolls normally — except at its very
   top, where pan-up only allows swiping up so a downward drag reaches the
   sheet instead of being swallowed by an overscroll attempt. */
.sheet-body { touch-action: pan-y; }
.sheet-body.body-at-top { touch-action: pan-up; }
.sheet-head { padding-top: 6px; }
.sheet-foot {
  padding: 10px 14px calc(12px + env(safe-area-inset-bottom));
  border-top: 1px solid var(--hairline);
}
.update-action { width: 100%; }

@keyframes sheet-up {
  from { transform: translateY(40px); opacity: 0; }
  to   { transform: translateY(0); opacity: 1; }
}

.update-pop-enter-active, .update-pop-leave-active { transition: opacity .14s ease, transform .14s ease; pointer-events: none; }
.update-pop-enter-from, .update-pop-leave-to { opacity: 0; }
.update-pop-enter-from.up, .update-pop-leave-to.up { transform: translateY(6px); }
.update-pop-enter-from.down, .update-pop-leave-to.down { transform: translateY(-6px); }

.update-sheet-enter-active, .update-sheet-leave-active { transition: opacity .18s ease; pointer-events: none; }
.update-sheet-enter-from, .update-sheet-leave-to { opacity: 0; }

@media (prefers-reduced-motion: reduce) {
  .update-pop-enter-active, .update-pop-leave-active,
  .update-sheet-enter-active, .update-sheet-leave-active { transition: none; }
  .update-sheet { animation: none; }
  .ring-indet { animation-duration: 2s; }
  .ring-fill { transition: none; }
}
</style>
