<template>
  <div v-if="visible" class="update-rail">
    <transition name="update-pop">
      <div v-if="open" class="update-pop" role="dialog" aria-label="Updates">
        <header class="update-head">
          <h3>{{ headline }}</h3>
          <span v-if="status?.phase === 'available' || status?.phase === 'ready'" class="update-version">
            {{ status.latest?.tag }}
          </span>
          <button class="update-close" type="button" aria-label="Schließen" @click="open = false">×</button>
        </header>

        <div class="update-body">
          <!-- checking -->
          <p v-if="status?.phase === 'checking'" class="update-hint"><span class="spin" /> Suche nach Updates…</p>

          <!-- up to date -->
          <template v-else-if="status?.phase === 'up_to_date'">
            <p class="update-hint ok">Alles aktuell – Version {{ status.current_version }}</p>
          </template>

          <!-- available: changelog since installed version -->
          <template v-else-if="status?.phase === 'available'">
            <p class="update-label">Was hat sich geändert</p>
            <ul class="update-notes">
              <li v-for="(change, i) in changelogLines" :key="i">{{ change }}</li>
            </ul>
            <button class="btn sm primary update-action" :disabled="busy" @click="download">
              Herunterladen
            </button>
          </template>

          <!-- downloading -->
          <p v-else-if="status?.phase === 'downloading'" class="update-hint">
            <span class="spin" /> Update wird heruntergeladen…
          </p>

          <!-- ready: install with restart -->
          <template v-else-if="status?.phase === 'ready'">
            <p class="update-hint">Update bereit ({{ shortDigest }}).</p>
            <p class="update-warn">Bei der Installation werden go2rtc und die Oberfläche neu gestartet.</p>
            <button class="btn sm primary update-action" :disabled="busy" @click="install">
              Jetzt aktualisieren
            </button>
          </template>

          <!-- installing -->
          <p v-else-if="status?.phase === 'installing'" class="update-hint">
            <span class="spin" /> Installation läuft – die Oberfläche startet gleich neu.
          </p>

          <!-- failed -->
          <template v-else-if="status?.phase === 'failed'">
            <p class="update-error">{{ status.error || 'Etwas ist schiefgelaufen.' }}</p>
            <button class="btn sm ghost update-action" :disabled="busy" @click="check">Erneut versuchen</button>
          </template>

          <!-- idle -->
          <template v-else>
            <p class="update-hint">Aktuelle Version: {{ status?.current_version || 'unbekannt' }}</p>
            <button class="btn sm ghost update-action" :disabled="busy" @click="check">Nach Updates suchen</button>
          </template>
        </div>
      </div>
    </transition>

    <button
      class="rail-update"
      :class="{ attention: hasUpdate, ready: isReady }"
      type="button"
      :aria-label="headline"
      :title="headline"
      @click="toggle"
    >
      <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <template v-if="isReady || hasUpdate">
          <path d="M12 3v12" /><path d="m7 10 5 5 5-5" /><path d="M4 21h16" />
        </template>
        <template v-else>
          <path d="M21 12a9 9 0 1 1-2.64-6.36" /><path d="M21 3v6h-6" />
        </template>
      </svg>
      <span v-if="hasUpdate || isReady" class="update-badge" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { api } from '../api/client'
import type { UpdateFlowStatus } from '../types'

const props = defineProps<{ visible: boolean }>()

const open = ref(false)
const status = ref<UpdateFlowStatus>()
const busy = ref(false)
let started = false
let pollTimer = 0
let autoCheckTimer = 0

const AUTO_CHECK_INTERVAL = 6 * 60 * 60 * 1000

const hasUpdate = computed(() => status.value?.phase === 'available')
const isReady = computed(() => status.value?.phase === 'ready')

const headline = computed(() => {
  switch (status.value?.phase) {
    case 'available':
      return 'Update verfügbar'
    case 'downloading':
      return 'Update wird geladen'
    case 'ready':
      return 'Update bereit zur Installation'
    case 'installing':
      return 'Installation läuft'
    case 'failed':
      return 'Update fehlgeschlagen'
    case 'up_to_date':
      return 'Kein Update verfügbar'
    default:
      return 'Updates'
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

function toggle() {
  open.value = !open.value
  if (open.value) void refresh()
}

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
}, { immediate: true })

onBeforeUnmount(() => {
  window.clearInterval(pollTimer)
  window.clearInterval(autoCheckTimer)
})
</script>

<style scoped>
.update-rail {
  position: relative;
  display: grid;
  justify-items: center;
}
.rail-update {
  position: relative;
  display: grid;
  place-items: center;
  width: 38px;
  height: 38px;
  border-radius: 50%;
  border: 1px solid var(--border, rgba(255, 255, 255, .08));
  background: var(--panel, #14141a);
  color: var(--muted, #9a9aa4);
  cursor: pointer;
}
.rail-update:hover,
.rail-update.attention,
.rail-update.ready {
  color: var(--accent, #6ea8ff);
}
.rail-update.attention,
.rail-update.ready {
  border-color: color-mix(in srgb, var(--accent, #6ea8ff) 55%, transparent);
}
.update-badge {
  position: absolute;
  top: -1px;
  right: -1px;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--accent, #6ea8ff);
  box-shadow: 0 0 0 2px var(--bg, #0b0b10);
}
.update-pop {
  position: absolute;
  bottom: 48px;
  left: 50%;
  transform: translateX(-50%);
  width: min(320px, 78vw);
  max-height: min(60vh, 480px);
  overflow: hidden auto;
  z-index: 60;
  border: 1px solid var(--accent, #6ea8ff);
  border-radius: 14px;
  background: var(--panel, #14141a);
  box-shadow: 0 18px 48px rgba(0, 0, 0, .5);
}
.update-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 12px 14px 8px;
}
.update-head h3 {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: .01em;
}
.update-version {
  font-size: 11px;
  color: var(--accent, #6ea8ff);
  font-family: ui-monospace, monospace;
}
.update-close {
  margin-left: auto;
  border: 0;
  background: none;
  color: inherit;
  opacity: .6;
  cursor: pointer;
  font-size: 16px;
  line-height: 1;
}
.update-close:hover { opacity: 1; }
.update-body {
  padding: 0 14px 14px;
  display: grid;
  gap: 10px;
}
.update-hint { margin: 0; font-size: 12.5px; color: var(--muted, #9a9aa4); display: flex; align-items: center; gap: 8px; }
.update-hint.ok { color: #79c98b; }
.update-label {
  margin: 0;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: .12em;
  color: var(--muted, #9a9aa4);
}
.update-notes {
  margin: 0;
  padding-left: 16px;
  display: grid;
  gap: 5px;
  font-size: 12.5px;
  color: var(--text, #dcdce2);
}
.update-notes li { overflow-wrap: anywhere; }
.update-warn { margin: 0; font-size: 12px; color: #ffb020; }
.update-error { margin: 0; font-size: 12.5px; color: #ff7a7a; overflow-wrap: anywhere; }
.update-action { justify-self: start; }
.spin {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  border: 2px solid var(--muted, #9a9aa4);
  border-top-color: transparent;
  animation: update-spin .8s linear infinite;
}
@keyframes update-spin { to { transform: rotate(360deg); } }
.update-pop-enter-active, .update-pop-leave-active { transition: opacity .15s ease, transform .15s ease; }
.update-pop-enter-from, .update-pop-leave-to { opacity: 0; transform: translateX(-50%) translateY(6px); }
</style>
