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
      :aria-expanded="open ? 'true' : 'false'"
      @click="toggle"
    >
      <!-- Download arrow only for a real pending release, refresh otherwise. -->
      <svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <template v-if="showDownloadIcon">
          <path d="M12 3v12" /><path d="m7 10 5 5 5-5" /><path d="M4 21h16" />
        </template>
        <template v-else>
          <path d="M21 12a9 9 0 1 1-2.64-6.36" /><path d="M21 3v6h-6" />
        </template>
      </svg>
      <span v-if="showBadge" class="update-badge" />
    </button>

    <Teleport to="body">
      <transition name="update-pop">
        <div
          v-if="open"
          ref="popEl"
          class="update-pop"
          :class="placement"
          :style="popStyle"
          role="dialog"
          aria-modal="false"
          :aria-label="headline"
        >
          <header class="update-head">
            <h3>{{ headline }}</h3>
            <span v-if="versionChip" class="update-version">{{ versionChip }}</span>
            <button
              class="update-icon-btn"
              type="button"
              aria-label="Nach Updates suchen"
              title="Nach Updates suchen"
              :disabled="busy || isWorking"
              @click="check"
            >
              <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M21 12a9 9 0 1 1-2.64-6.36" /><path d="M21 3v6h-6" />
              </svg>
            </button>
            <button class="update-icon-btn" type="button" aria-label="Schließen" title="Schließen" @click="close">
              <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" aria-hidden="true">
                <path d="M6 6l12 12M18 6 6 18" />
              </svg>
            </button>
          </header>

          <!-- Only this region scrolls, so the action below stays reachable
               no matter how long the changelog is. -->
          <div class="update-body">
            <!-- checking -->
            <p v-if="phase === 'checking'" class="update-hint"><span class="spin" /> Suche nach Updates…</p>

            <!-- up to date -->
            <template v-else-if="phase === 'up_to_date'">
              <p class="update-hint ok">Version {{ status?.current_version || 'unbekannt' }}</p>
              <p v-if="checkedLabel" class="update-meta">Zuletzt geprüft {{ checkedLabel }}</p>
            </template>

            <!-- available: changelog since the installed version -->
            <template v-else-if="phase === 'available'">
              <p class="update-hint">Installiert {{ status?.current_version || 'unbekannt' }} · neu {{ status?.latest?.tag }}</p>
              <p class="update-label">Was hat sich geändert</p>
              <ul class="update-notes">
                <li v-for="(change, i) in changelogLines" :key="i">{{ change }}</li>
              </ul>
            </template>

            <!-- downloading -->
            <p v-else-if="phase === 'downloading'" class="update-hint">
              <span class="spin" /> Update wird heruntergeladen…
            </p>

            <!-- ready: install with restart -->
            <template v-else-if="phase === 'ready'">
              <p class="update-hint">Update bereit ({{ shortDigest }}).</p>
              <p class="update-warn">Bei der Installation werden go2rtc und die Oberfläche neu gestartet.</p>
            </template>

            <!-- installing -->
            <p v-else-if="phase === 'installing'" class="update-hint">
              <span class="spin" /> Installation läuft – die Oberfläche startet gleich neu.
            </p>

            <!-- failed -->
            <p v-else-if="phase === 'failed'" class="update-error">{{ status?.error || 'Etwas ist schiefgelaufen.' }}</p>

            <!-- idle -->
            <p v-else class="update-hint">Version {{ status?.current_version || 'unbekannt' }}</p>
          </div>

          <footer v-if="action" class="update-foot">
            <button
              class="btn sm update-action"
              :class="action.variant"
              type="button"
              :disabled="busy"
              @click="action.run"
            >
              {{ action.label }}
            </button>
          </footer>
        </div>
      </transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { api } from '../api/client'
import type { UpdateFlowPhase, UpdateFlowStatus } from '../types'

const props = defineProps<{ visible: boolean }>()

const open = ref(false)
const status = ref<UpdateFlowStatus>()
const busy = ref(false)
const triggerEl = ref<HTMLButtonElement>()
const popEl = ref<HTMLElement>()
let started = false
let pollTimer = 0
let autoCheckTimer = 0

const AUTO_CHECK_INTERVAL = 6 * 60 * 60 * 1000
const POP_WIDTH = 340
const POP_GAP = 10
const VIEWPORT_MARGIN = 12
const MIN_POP_HEIGHT = 180
// Keep the panel a popover, not a full-height sheet, even with a long changelog.
const MAX_POP_HEIGHT = 460

function normalizeVersion(value: string | undefined): string {
  return (value ?? '').trim().replace(/^v/i, '')
}

// The backend already filters out releases that are not newer, but the whole UI
// must never advertise an update for the version that is already installed.
const hasNewerRelease = computed(() => {
  const tag = normalizeVersion(status.value?.latest?.tag)
  return tag !== '' && tag !== normalizeVersion(status.value?.current_version)
})

// Single source of truth: heading, body, icon and badge all read this.
const phase = computed<UpdateFlowPhase>(() => {
  const raw = status.value?.phase ?? 'idle'
  if (raw === 'available' && !hasNewerRelease.value) return 'up_to_date'
  return raw
})

const hasUpdate = computed(() => phase.value === 'available')
const isReady = computed(() => phase.value === 'ready')
const isWorking = computed(() => phase.value === 'downloading' || phase.value === 'installing')
const showDownloadIcon = computed(() => hasUpdate.value || isReady.value)
const showBadge = computed(() => showDownloadIcon.value)

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

const triggerLabel = computed(() => (open.value ? 'Updates schließen' : headline.value))

const versionChip = computed(() => {
  if (phase.value === 'available' || phase.value === 'ready') return status.value?.latest?.tag ?? ''
  return ''
})

const checkedLabel = computed(() => {
  const raw = status.value?.checked_at
  if (!raw) return ''
  const at = new Date(raw)
  if (Number.isNaN(at.getTime())) return ''
  return at.toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' })
})

// The single action offered for the current phase, pinned below the scroll area.
const action = computed<{ label: string; variant: string; run: () => void } | null>(() => {
  switch (phase.value) {
    case 'available':
      return { label: 'Herunterladen', variant: 'primary', run: download }
    case 'ready':
      return { label: 'Jetzt aktualisieren', variant: 'primary', run: install }
    case 'failed':
      return { label: 'Erneut versuchen', variant: 'ghost', run: check }
    case 'idle':
      return { label: 'Nach Updates suchen', variant: 'ghost', run: check }
    default:
      return null
  }
})

const shortDigest = computed(() => {
  const digest = status.value?.digest ?? ''
  return digest ? digest.slice(0, 8) + '…' : ''
})

const changelogLines = computed<string[]>(() => {
  const lines: string[] = []
  for (const release of status.value?.changes ?? []) {
    if (release.tag) lines.push(`${release.tag}`)
    for (const raw of (release.notes ?? '').split('\n')) {
      const line = cleanNoteLine(raw)
      if (line) lines.push(line)
    }
  }
  // Deduplicate consecutive duplicates and cap the list length.
  const out: string[] = []
  for (const line of lines) {
    if (out.length >= 40) break
    if (out[out.length - 1] !== line) out.push(line)
  }
  return out
})

function cleanNoteLine(raw: string): string {
  let line = raw.trim()
  if (!line) return ''
  if (/^!\[/.test(line)) return '' // images
  line = line.replace(/^[-*+]\s+/, '')
  line = line.replace(/^#+\s+/, '')
  line = line.replace(/!\[[^\]]*\]\([^)]*\)/g, '')
  line = line.replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
  line = line.replace(/`/g, '')
  line = line.trim()
  if (!line) return ''
  return line.length > 160 ? line.slice(0, 157) + '…' : line
}

/* ---------- Popover placement ------------------------------------------
   The rail is a narrow, sticky column, so the popover is teleported to the
   body and positioned from the trigger rect: it opens upward whenever there
   is room and is always clamped into the viewport. */

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

function onDocumentPointerDown(event: MouseEvent) {
  const target = event.target as Node | null
  if (!target) return
  if (popEl.value?.contains(target) || triggerEl.value?.contains(target)) return
  close()
}

function onDocumentKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    close()
    triggerEl.value?.focus()
  }
}

function bindPopListeners() {
  window.addEventListener('resize', positionPop)
  window.addEventListener('scroll', positionPop, true)
  document.addEventListener('mousedown', onDocumentPointerDown)
  document.addEventListener('keydown', onDocumentKeydown)
}

function unbindPopListeners() {
  window.removeEventListener('resize', positionPop)
  window.removeEventListener('scroll', positionPop, true)
  document.removeEventListener('mousedown', onDocumentPointerDown)
  document.removeEventListener('keydown', onDocumentKeydown)
}

async function refresh() {
  try {
    status.value = await api.getUpdateStatus()
  } catch {
    /* keep last state */
  }
  schedulePoll()
}

async function check() {
  busy.value = true
  try {
    status.value = await api.checkForUpdates()
  } catch {
    await refresh()
  } finally {
    busy.value = false
    schedulePoll()
  }
}

async function download() {
  busy.value = true
  try {
    status.value = await api.downloadUpdate()
  } catch (err) {
    status.value = { ...(status.value ?? { phase: 'idle', current_version: '' }), phase: 'failed', error: err instanceof Error ? err.message : 'Download fehlgeschlagen.' }
  } finally {
    busy.value = false
    schedulePoll()
  }
}

async function install() {
  busy.value = true
  try {
    await api.installUpdate()
    await refresh()
  } catch (err) {
    status.value = { ...(status.value ?? { phase: 'idle', current_version: '' }), phase: 'failed', error: err instanceof Error ? err.message : 'Installation fehlgeschlagen.' }
  } finally {
    busy.value = false
    schedulePoll()
  }
}

function close() {
  open.value = false
}

function toggle() {
  open.value = !open.value
  if (open.value) void refresh()
}

watch(open, async (isOpen) => {
  if (!isOpen) {
    unbindPopListeners()
    return
  }
  positionPop()
  bindPopListeners()
  await nextTick()
  positionPop()
})

// The changelog changes the popover height, so re-anchor when the phase flips.
watch(phase, () => {
  if (open.value) void nextTick(positionPop)
})

// Poll while a background phase runs; otherwise stay passive.
function schedulePoll() {
  window.clearInterval(pollTimer)
  if (status.value?.phase === 'downloading' || status.value?.phase === 'installing') {
    pollTimer = window.setInterval(() => void refresh(), 2000)
  }
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
  else close()
}, { immediate: true })

onBeforeUnmount(() => {
  window.clearInterval(pollTimer)
  window.clearInterval(autoCheckTimer)
  unbindPopListeners()
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
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--ink-dim);
  cursor: pointer;
}
.update-icon-btn:first-of-type { margin-left: auto; }
.update-icon-btn:hover:not(:disabled) { color: var(--ink); background: var(--raised); }
.update-icon-btn:disabled { opacity: .45; cursor: not-allowed; }
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
.update-hint { margin: 0; font-size: 12px; color: var(--ink-soft); display: flex; align-items: center; gap: 8px; overflow-wrap: anywhere; }
.update-hint.ok { color: var(--live); }
.update-meta { margin: 0; font-size: 10.5px; color: var(--ink-dim); letter-spacing: 0.05em; }
.update-label {
  margin: 0;
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.12em;
  color: var(--ink-dim);
}
.update-notes {
  margin: 0;
  padding-left: 16px;
  display: grid;
  gap: 5px;
  font-size: 12px;
  color: var(--ink-soft);
  line-height: 1.5;
}
.update-notes li { overflow-wrap: anywhere; }
.update-warn { margin: 0; font-size: 11.5px; color: var(--warn); line-height: 1.5; }
.update-error { margin: 0; font-size: 12px; color: var(--danger); overflow-wrap: anywhere; }
.update-foot {
  padding: 10px 14px 12px;
  border-top: 1px solid var(--hairline);
}
.update-action { width: 100%; }
.spin {
  width: 11px;
  height: 11px;
  flex: 0 0 auto;
  border-radius: 999px;
  border: 2px solid var(--ink-dim);
  border-top-color: transparent;
  animation: update-spin .8s linear infinite;
}
@keyframes update-spin { to { transform: rotate(360deg); } }

.update-pop-enter-active, .update-pop-leave-active { transition: opacity .14s ease, transform .14s ease; }
.update-pop-enter-from, .update-pop-leave-to { opacity: 0; }
.update-pop-enter-from.up, .update-pop-leave-to.up { transform: translateY(6px); }
.update-pop-enter-from.down, .update-pop-leave-to.down { transform: translateY(-6px); }

@media (prefers-reduced-motion: reduce) {
  .update-pop-enter-active, .update-pop-leave-active { transition: none; }
  .spin { animation-duration: 2s; }
}
</style>
