<template>
  <header class="topline">
    <div>
      <div class="eyebrow">Watchdeck · Übersicht</div>
      <h1 class="headline">
        <template v-if="loading">Wird geladen<em>…</em></template>
        <template v-else-if="allOnline">Alles<em> in Ordnung.</em></template>
        <template v-else-if="anyOffline"><span class="accent" style="color: var(--warn)">{{ offlineCount }}</span> {{ offlineCount === 1 ? 'Kamera' : 'Kameras' }} <em>fehlen.</em></template>
        <template v-else>Bereit zur <em>Einrichtung.</em></template>
      </h1>
    </div>
    <div class="meta">
      <div>Letzte Suche · <b>{{ lastScanRel }}</b></div>
      <div>Lokale Adresse · <b>127.0.0.1:8091</b></div>
    </div>
  </header>

  <div v-if="error" class="notice err"><span class="tag">FEHLER</span>{{ error }}</div>

  <!-- Primary actions -->
  <div class="panel flush" style="background: transparent; border: 0; padding: 0;">
    <div class="btn-row">
      <a class="btn live lg" href="http://localhost:8090/" target="_blank">Kameras öffnen ↗</a>
      <button class="btn lg" :disabled="!!busy" @click="runDiscovery">{{ busy === 'scan' ? 'Suche läuft…' : 'Neu suchen' }}</button>
      <button class="btn ghost" :disabled="!!busy" @click="restart">Server neu starten</button>
      <div class="spacer" />
      <RouterLink class="btn ghost" to="/einrichtung">Einrichtung →</RouterLink>
    </div>
  </div>

  <!-- Service strip -->
  <section class="service-strip fade-in">
    <div class="service">
      <div>
        <div class="name">AgentDVR</div>
        <div class="endpoint">localhost:8090 · Anzeige</div>
      </div>
      <span class="pill" :class="status?.system.agentdvr.online ? 'live' : 'down'">
        {{ status?.system.agentdvr.online ? 'aktiv' : 'offline' }}
      </span>
    </div>
    <div class="service">
      <div>
        <div class="name">go2rtc</div>
        <div class="endpoint">localhost:1984 · Stream-Alias</div>
      </div>
      <span class="pill" :class="status?.system.go2rtc.online ? 'live' : 'down'">
        {{ status?.system.go2rtc.online ? 'aktiv' : 'offline' }}
      </span>
    </div>
    <div class="service">
      <div>
        <div class="name">Manager</div>
        <div class="endpoint">camera-appliance · lokal</div>
      </div>
      <span class="pill live">aktiv</span>
    </div>
  </section>

  <!-- Stats -->
  <section class="stat-row fade-in">
    <div class="stat">
      <div class="label">Plätze</div>
      <div class="value">{{ slotsCount.toString().padStart(2, '0') }}</div>
      <div class="sub">konfiguriert</div>
    </div>
    <div class="stat">
      <div class="label">Zugeordnet</div>
      <div class="value live">{{ boundCount.toString().padStart(2, '0') }}</div>
      <div class="sub">von {{ slotsCount }}</div>
    </div>
    <div class="stat">
      <div class="label">Geräte gefunden</div>
      <div class="value">{{ deviceCount.toString().padStart(2, '0') }}</div>
      <div class="sub">im Netzwerk</div>
    </div>
    <div class="stat">
      <div class="label">Offen</div>
      <div class="value" :class="{ warn: missing > 0 }">{{ missing.toString().padStart(2, '0') }}</div>
      <div class="sub">{{ missing === 0 ? 'nichts zu tun' : 'Plätze noch leer' }}</div>
    </div>
  </section>

  <!-- Live grid + events -->
  <div class="split-3-2 fade-in">
    <section class="panel">
      <div class="panel-head">
        <h2>Anzeige · Live-Belegung</h2>
        <div class="right">{{ boundCount }}/{{ slotsCount }} aktiv</div>
      </div>
      <div class="live-grid">
        <div
          v-for="slot in slots"
          :key="slot.id"
          class="live-slot"
          :class="{
            large: slot.role === 'large',
            on: stateFor(slot.id) === 'on',
            off: stateFor(slot.id) === 'off',
            empty: stateFor(slot.id) === 'empty'
          }"
        >
          <img
            v-if="referenceVisible(deviceIdFor(slot.id))"
            class="slot-reference"
            :src="referenceImageUrl(deviceIdFor(slot.id))"
            alt=""
            @error="markReferenceMissing(deviceIdFor(slot.id))"
          />
          <div class="scan-overlay" />
          <div class="slot-tag">
            <span>{{ slot.id }}</span>
            <span class="pill" :class="pillClass(stateFor(slot.id))">{{ stateLabel(stateFor(slot.id)) }}</span>
          </div>
          <div>
            <div class="slot-name">{{ bindingFor(slot.id)?.label || slot.label }}</div>
            <div class="slot-meta">
              <span class="ip">{{ bindingFor(slot.id)?.device?.last_ip || '— nicht zugeordnet —' }}</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>Ereignisprotokoll</h2>
        <RouterLink class="right" to="/system" style="text-decoration: underline; text-underline-offset: 3px;">Alle ansehen</RouterLink>
      </div>
      <div v-if="!status?.recent_events?.length" class="empty">Noch keine Ereignisse.</div>
      <div v-else class="ticker">
        <div v-for="ev in status.recent_events.slice(0, 7)" :key="ev.id" class="row">
          <span class="time">{{ formatTime(ev.created_at) }}</span>
          <span class="lvl" :class="levelClass(ev.level)">{{ ev.level }}</span>
          <span>{{ ev.message }}</span>
        </div>
      </div>
    </section>
  </div>

  <div class="toast-host">
    <transition name="page"><div v-if="toast" class="toast" :key="toast">{{ toast }}</div></transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api/client'
import type { StatusResponse } from '../types'

const status = ref<StatusResponse>()
const loading = ref(true)
const busy = ref<'' | 'scan' | 'restart'>('')
const error = ref('')
const toast = ref('')
const frameRevision = ref(Date.now())
const missingReferences = ref<Record<string, boolean>>({})

const slots = computed(() => status.value?.slots ?? [])
const slotsCount = computed(() => slots.value.length)
const boundCount = computed(() => slots.value.filter((s) => bindingFor(s.id)?.device?.last_ip).length)
const deviceCount = computed(() => status.value?.devices.length ?? 0)
const missing = computed(() => Math.max(0, slotsCount.value - boundCount.value))
const offlineCount = computed(() => slots.value.filter((s) => bindingFor(s.id) && !bindingFor(s.id)?.device?.last_ip).length)
const allOnline = computed(() => slotsCount.value > 0 && missing.value === 0 && offlineCount.value === 0)
const anyOffline = computed(() => offlineCount.value > 0 || missing.value > 0)

const lastScanRel = computed(() => {
  const t = status.value?.scan_runs?.[0]?.started_at
  if (!t) return 'nie'
  const diff = (Date.now() - new Date(t).getTime()) / 1000
  if (diff < 60) return 'gerade eben'
  if (diff < 3600) return `vor ${Math.floor(diff / 60)} Min.`
  if (diff < 86400) return `vor ${Math.floor(diff / 3600)} Std.`
  return new Date(t).toLocaleDateString('de-DE')
})

function bindingFor(slotId: string) {
  return status.value?.bindings.find((b) => b.slot_id === slotId)
}
function deviceIdFor(slotId: string) {
  return bindingFor(slotId)?.device_id
}
function referenceVisible(deviceId?: string) {
  return Boolean(deviceId && !missingReferences.value[deviceId])
}
function referenceImageUrl(deviceId?: string) {
  return deviceId ? api.referenceImageUrl(deviceId, frameRevision.value) : ''
}
function markReferenceMissing(deviceId?: string) {
  if (!deviceId) return
  missingReferences.value = { ...missingReferences.value, [deviceId]: true }
}
function stateFor(slotId: string): 'on' | 'off' | 'empty' {
  const b = bindingFor(slotId)
  if (!b) return 'empty'
  return b.device?.last_ip ? 'on' : 'off'
}
function stateLabel(s: 'on' | 'off' | 'empty') {
  return s === 'on' ? 'live' : s === 'off' ? 'offline' : 'leer'
}
function pillClass(s: 'on' | 'off' | 'empty') {
  return s === 'on' ? 'live' : s === 'off' ? 'down' : ''
}
function levelClass(l: string) {
  const lower = (l || '').toLowerCase()
  if (lower.includes('err') || lower.includes('fail')) return 'err'
  if (lower.includes('warn')) return 'warn'
  if (lower.includes('ok') || lower.includes('info')) return 'ok'
  return ''
}
function formatTime(t: string) {
  return new Date(t).toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' })
}

async function load() {
  try {
    status.value = await api.status()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Status konnte nicht geladen werden.'
  } finally {
    loading.value = false
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

async function restart() {
  busy.value = 'restart'
  try {
    await api.restartStack()
    toast.value = 'Server wurde neu gestartet'
    setTimeout(() => (toast.value = ''), 2400)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Server konnte nicht neu gestartet werden.'
  } finally {
    busy.value = ''
  }
}

onMounted(load)
</script>
