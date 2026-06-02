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
    <button class="btn" :disabled="!!busy" @click="runDiscovery">{{ busy === 'scan' ? 'Suche läuft…' : 'Kameras suchen' }}</button>
    <button class="btn" :disabled="!!busy" @click="render">{{ busy === 'render' ? 'Erzeugt…' : 'go2rtc erzeugen' }}</button>
    <button class="btn ghost" :disabled="!!busy" @click="restartGo2rtc">{{ busy === 'restart' ? 'Startet…' : 'go2rtc neu starten' }}</button>
    <div class="spacer" />
    <RouterLink class="btn ghost" to="/einrichtung">Einrichtung</RouterLink>
  </div>

  <section class="viewer-grid">
    <article
      v-for="slot in slots"
      :key="slot.alias"
      class="viewer-tile"
      :class="[tileClass(slot), { large: slot.slot.role === 'large' }]"
    >
      <div class="viewer-frame-wrap">
        <iframe
          v-if="slot.playback?.page_url && isPlayable(slot)"
          class="viewer-frame"
          :src="slot.playback.page_url"
          :title="slot.label"
          allow="autoplay; fullscreen; picture-in-picture"
          @load="markFrameReady(slot.alias)"
        />
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
        <RouterLink class="action" :to="slot.binding?.device_id ? `/kamera/${slot.binding.device_id}` : '/einrichtung'">
          Öffnen
        </RouterLink>
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
import type { ViewerResponse, ViewerSlot, ViewerSlotState } from '../types'

const viewer = ref<ViewerResponse>()
const loading = ref(true)
const busy = ref<'' | 'load' | 'scan' | 'render' | 'restart'>('')
const error = ref('')
const toast = ref('')
const frameReady = ref<Record<string, boolean>>({})
let refreshTimer = 0

const slots = computed(() => viewer.value?.slots ?? [])
const onlineCount = computed(() => slots.value.filter((slot) => effectiveState(slot) === 'online').length)
const blockingSlots = computed(() => slots.value.filter((slot) => !['online', 'connecting'].includes(effectiveState(slot))))
const blockingCount = computed(() => blockingSlots.value.length)
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
  return slot.path.kind === 'direct' ? 'direkt' : `Relay ${slot.path.label}`
}

async function load() {
  busy.value = busy.value || 'load'
  error.value = ''
  try {
    viewer.value = await api.viewer()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Viewer konnte nicht geladen werden.'
  } finally {
    loading.value = false
    if (busy.value === 'load') busy.value = ''
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
  void load()
  refreshTimer = window.setInterval(() => {
    if (!busy.value) void load()
  }, 15000)
})

onBeforeUnmount(() => {
  window.clearInterval(refreshTimer)
})
</script>
