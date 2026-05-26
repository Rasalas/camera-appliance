<template>
  <PageHeader title="Kameras suchen" subtitle="Erst finden, dann in der Kamera-Ansicht zuordnen">
    <button class="action-button primary" :disabled="running" @click="run">{{ running ? 'Suche läuft...' : 'Suche starten' }}</button>
  </PageHeader>
  <ErrorMessage :message="error" />

  <div class="setup-steps">
    <span class="active">1 Suche starten</span>
    <span>2 Kandidat prüfen</span>
    <span>3 Anzeigeplatz wählen</span>
  </div>

  <Card v-if="running">
    <h2>Netzwerk wird durchsucht</h2>
    <div class="progress"><span /></div>
    <p>Es werden erreichbare Geräte, RTSP, ONVIF und Tapo/TP-Link-Signale geprüft.</p>
  </Card>

  <Card>
    <div class="card-row">
      <div>
        <h2>Gefundene Kandidaten</h2>
        <p v-if="subnets.length">{{ subnets.map((subnet) => `${subnet.cidr} über ${subnet.interface}`).join(', ') }}</p>
        <p v-else>Starte eine Suche im lokalen Netzwerk.</p>
      </div>
      <button v-if="ignoredIds.length" class="action-button secondary" @click="showIgnored = !showIgnored">
        {{ showIgnored ? 'Ausgeblendete verstecken' : `Ausgeblendete anzeigen (${ignoredIds.length})` }}
      </button>
    </div>

    <div class="candidate-list">
      <article v-for="device in visibleDevices" :key="device.id" class="candidate-row">
        <div>
          <strong>{{ deviceTitle(device) }}</strong>
          <p>{{ deviceHint(device) }}</p>
          <span class="mono">{{ device.last_ip || 'IP unbekannt' }}</span>
        </div>
        <div class="signal-grid compact-signals">
          <span :class="signalClass(device, 'rtsp_port_open')">RTSP</span>
          <span :class="signalClass(device, 'onvif_port_open')">ONVIF</span>
          <span :class="httpClass(device)">HTTP</span>
        </div>
        <div class="candidate-actions">
          <RouterLink class="button-link" :to="`/bindings?device=${device.id}`">Auswählen</RouterLink>
          <RouterLink class="button-link secondary" :to="`/devices/${device.id}`">Prüfen</RouterLink>
          <button class="action-button secondary" @click="ignore(device.id)">Ausblenden</button>
        </div>
      </article>
    </div>

    <EmptyState v-if="!running && visibleDevices.length === 0" text="Keine Kandidaten sichtbar. Starte eine Suche oder zeige ausgeblendete Geräte an." />
  </Card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api/client'
import type { Device } from '../types'
import PageHeader from '../components/PageHeader.vue'
import Card from '../components/Card.vue'
import EmptyState from '../components/EmptyState.vue'
import ErrorMessage from '../components/ErrorMessage.vue'

const running = ref(false)
const devices = ref<Device[]>([])
const subnets = ref<Array<{ cidr: string; interface: string }>>([])
const error = ref('')
const showIgnored = ref(false)
const ignoredIds = ref<string[]>(JSON.parse(localStorage.getItem('ignoredDeviceIds') || '[]') as string[])
const visibleDevices = computed(() => devices.value.filter((device) => showIgnored.value || !ignoredIds.value.includes(device.id)))

async function run() {
  running.value = true
  error.value = ''
  try {
    const result = await api.discover()
    devices.value = result.devices
    subnets.value = result.subnets
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Kamerasuche fehlgeschlagen.'
  } finally {
    running.value = false
  }
}

onMounted(async () => {
  const status = await api.status()
  devices.value = status.devices
  subnets.value = status.scan_runs.length ? [] : []
})

function ignore(id: string) {
  if (!ignoredIds.value.includes(id)) {
    ignoredIds.value = [...ignoredIds.value, id]
    localStorage.setItem('ignoredDeviceIds', JSON.stringify(ignoredIds.value))
  }
}

function deviceTitle(device: Device) {
  return `${device.manufacturer || 'Unbekanntes'} ${device.model || 'Gerät'}`.trim()
}

function raw(device: Device) {
  if (!device.raw_json) return {} as Record<string, unknown>
  if (typeof device.raw_json === 'string') {
    try { return JSON.parse(device.raw_json) as Record<string, unknown> } catch { return {} as Record<string, unknown> }
  }
  return device.raw_json
}

function deviceHint(device: Device) {
  const info = raw(device)
  if (info.rtsp_port_open) return 'Kamera-Stream wahrscheinlich nutzbar.'
  if (info.onvif_port_open) return 'Kamera-Kandidat, RTSP muss eventuell aktiviert werden.'
  if (String(info.http_signature || '').includes('SHIP 2.0')) return 'TP-Link/Tapo-Kandidat ohne offenen RTSP-Port.'
  return 'Unklarer Kandidat. Erst prüfen oder ausblenden.'
}

function signalClass(device: Device, key: string) {
  return raw(device)[key] ? 'signal ok' : 'signal muted'
}

function httpClass(device: Device) {
  return raw(device).http_signature ? 'signal ok' : 'signal muted'
}
</script>
