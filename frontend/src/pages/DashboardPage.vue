<template>
  <PageHeader title="Camera Appliance" subtitle="Lokale Kamera-Anzeige und Verwaltung">
    <StatusBadge :online="true" />
  </PageHeader>
  <ErrorMessage :message="error" />
  <div class="button-row hero-actions">
    <a class="action-button primary" href="http://localhost:8090/" target="_blank">Kameras öffnen</a>
    <button class="action-button secondary" :disabled="busy" @click="runDiscovery">Kameras neu suchen</button>
    <button class="action-button secondary" :disabled="busy" @click="restart">Server neu starten</button>
  </div>
  <LoadingState v-if="loading" />
  <template v-else-if="status">
    <div class="grid two">
      <Card>
        <h2>Systemstatus</h2>
        <div class="status-line"><span>AgentDVR</span><StatusBadge :online="status.system.agentdvr.online" /></div>
        <div class="status-line"><span>go2rtc</span><StatusBadge :online="status.system.go2rtc.online" /></div>
        <div class="status-line"><span>Manager</span><StatusBadge :online="status.system.camera_appliance.online" /></div>
        <div class="status-line"><span>Letzte Suche</span><span>{{ lastScan }}</span></div>
      </Card>
      <Card>
        <h2>Kameras</h2>
        <div v-for="slot in status.slots" :key="slot.id" class="status-line">
          <span>{{ slot.id }} {{ bindingFor(slot.id)?.label || slot.label }}</span>
          <StatusBadge :online="Boolean(bindingFor(slot.id)?.device?.last_ip)" />
        </div>
      </Card>
    </div>
    <Card>
      <h2>Letzte Ereignisse</h2>
      <EmptyState v-if="status.recent_events.length === 0" text="Noch keine Ereignisse vorhanden." />
      <ul class="event-list">
        <li v-for="event in status.recent_events" :key="event.id">{{ formatDate(event.created_at) }} - {{ event.message }}</li>
      </ul>
    </Card>
  </template>
  <Toast :message="toast" />
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api/client'
import type { StatusResponse } from '../types'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import Card from '../components/Card.vue'
import LoadingState from '../components/LoadingState.vue'
import ErrorMessage from '../components/ErrorMessage.vue'
import EmptyState from '../components/EmptyState.vue'
import Toast from '../components/Toast.vue'

const status = ref<StatusResponse>()
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const toast = ref('')

const lastScan = computed(() => status.value?.scan_runs?.[0]?.message || 'keine Suche')

function bindingFor(slotId: string) {
  return status.value?.bindings.find((binding) => binding.slot_id === slotId)
}

function formatDate(value: string) {
  return new Date(value).toLocaleString('de-DE')
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    status.value = await api.status()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Status konnte nicht geladen werden.'
  } finally {
    loading.value = false
  }
}

async function runDiscovery() {
  busy.value = true
  try {
    const result = await api.discover()
    toast.value = `${result.devices.length} Gerät(e) gefunden`
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Kamerasuche fehlgeschlagen.'
  } finally {
    busy.value = false
  }
}

async function restart() {
  busy.value = true
  try {
    await api.restartStack()
    toast.value = 'Server wurde neu gestartet'
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Server konnte nicht neu gestartet werden.'
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>
