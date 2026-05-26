<template>
  <PageHeader title="Kameras" subtitle="Alle gefundenen Kamera-Ressourcen">
    <button class="action-button primary" :disabled="running" @click="runDiscovery">{{ running ? 'Suche läuft...' : 'Kameras suchen' }}</button>
  </PageHeader>
  <ErrorMessage :message="error" />

  <Card v-if="running">
    <h2>Suche läuft</h2>
    <div class="progress"><span /></div>
    <p>Das lokale Netzwerk wird nach Kameras und Kamera-Kandidaten durchsucht.</p>
  </Card>

  <div class="resource-toolbar">
    <div>
      <strong>{{ visibleDevices.length }}</strong>
      <span>Kamera-Ressourcen</span>
    </div>
    <button v-if="ignoredIds.length" class="action-button secondary" @click="showIgnored = !showIgnored">
      {{ showIgnored ? 'Ausgeblendete verstecken' : `Ausgeblendete anzeigen (${ignoredIds.length})` }}
    </button>
  </div>

  <div class="resource-list">
    <article v-for="device in visibleDevices" :key="device.id" class="resource-row">
      <div class="resource-main">
        <strong>{{ deviceTitle(device) }}</strong>
        <span class="mono">{{ device.last_ip || 'IP unbekannt' }}</span>
        <small>{{ device.mac_address || 'MAC unbekannt' }}</small>
      </div>
      <div class="resource-state">
        <span :class="signalClass(device, 'rtsp_port_open')">RTSP</span>
        <span :class="signalClass(device, 'onvif_port_open')">ONVIF</span>
        <span :class="httpClass(device)">HTTP</span>
      </div>
      <div class="resource-actions">
        <RouterLink class="button-link" :to="`/cameras/${device.id}`">Details</RouterLink>
        <RouterLink class="button-link secondary" :to="`/display?camera=${device.id}`">Anzeige zuordnen</RouterLink>
        <button class="action-button secondary" @click="ignore(device.id)">Ausblenden</button>
      </div>
    </article>
  </div>

  <EmptyState v-if="!running && visibleDevices.length === 0" text="Noch keine Kameras gefunden. Starte die Suche." />
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
const error = ref('')
const showIgnored = ref(false)
const ignoredIds = ref<string[]>(JSON.parse(localStorage.getItem('ignoredDeviceIds') || '[]') as string[])
const visibleDevices = computed(() => devices.value.filter((device) => showIgnored.value || !ignoredIds.value.includes(device.id)))

async function load() {
  devices.value = await api.devices()
}

async function runDiscovery() {
  running.value = true
  error.value = ''
  try {
    const result = await api.discover()
    devices.value = result.devices
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Kamerasuche fehlgeschlagen.'
  } finally {
    running.value = false
  }
}

function ignore(id: string) {
  ignoredIds.value = [...new Set([...ignoredIds.value, id])]
  localStorage.setItem('ignoredDeviceIds', JSON.stringify(ignoredIds.value))
}

function raw(device: Device) {
  if (!device.raw_json) return {} as Record<string, unknown>
  if (typeof device.raw_json === 'string') {
    try { return JSON.parse(device.raw_json) as Record<string, unknown> } catch { return {} as Record<string, unknown> }
  }
  return device.raw_json
}

function deviceTitle(device: Device) {
  return `${device.manufacturer || 'Unbekannte'} ${device.model || 'Kamera'}`.trim()
}

function signalClass(device: Device, key: string) {
  return raw(device)[key] ? 'signal ok' : 'signal muted'
}

function httpClass(device: Device) {
  return raw(device).http_signature ? 'signal ok' : 'signal muted'
}

onMounted(load)
</script>
