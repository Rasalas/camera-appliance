<template>
  <PageHeader title="Kameras zuordnen" subtitle="Kandidat wählen, Platz anklicken, speichern" />
  <ErrorMessage :message="error" />

  <div class="setup-steps">
    <span>1 Kandidat wählen</span>
    <span :class="{ active: selectedDevice }">2 Platz anklicken</span>
    <span :class="{ active: selectedSlot }">3 Speichern</span>
  </div>

  <div class="assign-screen">
    <Card>
      <h2>Kandidaten</h2>
      <EmptyState v-if="availableDevices.length === 0" text="Keine freien Kandidaten. Starte zuerst eine Suche." />
      <div class="candidate-stack">
        <button
          v-for="device in availableDevices"
          :key="device.id"
          class="candidate-choice"
          :class="{ selected: selectedDevice?.id === device.id }"
          @click="selectDevice(device)"
        >
          <strong>{{ deviceTitle(device) }}</strong>
          <span>{{ device.last_ip || 'ohne IP' }}</span>
          <small>{{ deviceSignal(device) }}</small>
        </button>
      </div>
    </Card>

    <Card class="layout-card">
      <div class="card-row">
        <div>
          <h2>Anzeigeplätze</h2>
          <p>{{ selectedDevice ? 'Klicke den Platz für die ausgewählte Kamera.' : 'Wähle links zuerst eine Kamera aus.' }}</p>
        </div>
        <RouterLink class="button-link secondary" to="/discovery">Neu suchen</RouterLink>
      </div>
      <div class="camera-layout">
        <button v-for="slot in slots" :key="slot.id" class="layout-slot" :class="slotClass(slot.id)" @click="chooseSlot(slot.id)">
          <span class="slot-id">{{ slot.id }}</span>
          <strong>{{ bindingFor(slot.id)?.label || slot.label }}</strong>
          <small>{{ bindingFor(slot.id)?.device?.last_ip || 'frei' }}</small>
        </button>
      </div>
    </Card>
  </div>

  <Card v-if="selectedSlot" class="save-panel">
    <h2>{{ selectedSlot }} speichern</h2>
    <div class="grid two compact">
      <div>
        <p>Gerät</p>
        <strong>{{ selectedDevice ? deviceTitle(selectedDevice) : 'Kein Kandidat gewählt' }}</strong>
        <p v-if="selectedDevice" class="mono">{{ selectedDevice.last_ip }} · {{ selectedDevice.mac_address || 'MAC unbekannt' }}</p>
      </div>
      <div>
        <label>Anzeigename<input v-model="form.label" :placeholder="selectedSlotLabel" /></label>
        <label>Benutzername<input v-model="form.username" placeholder="tapo_hof" /></label>
        <label>Stream
          <select v-model="form.stream_name">
            <option value="stream2">stream2</option>
            <option value="stream1">stream1</option>
          </select>
        </label>
      </div>
    </div>
    <div class="button-row">
      <button class="action-button primary" :disabled="!selectedDevice || saving" @click="save">Zuordnung speichern</button>
      <RouterLink v-if="selectedDevice" class="button-link secondary" :to="`/devices/${selectedDevice.id}`">Kandidat prüfen</RouterLink>
      <button class="action-button danger" :disabled="!bindingFor(selectedSlot)" @click="remove">Zuordnung entfernen</button>
    </div>
  </Card>

  <Card>
    <div class="card-row">
      <div>
        <h2>go2rtc und AgentDVR</h2>
        <p>Nach dem Speichern Konfiguration erzeugen. AgentDVR nutzt weiter nur die stabilen Namen cam1 bis cam5.</p>
      </div>
      <div class="button-row">
        <button class="action-button secondary" @click="render">go2rtc erzeugen</button>
        <button class="action-button secondary" @click="restart">go2rtc neu starten</button>
      </div>
    </div>
    <pre v-if="rendered">{{ rendered }}</pre>
  </Card>
  <Toast :message="toast" />
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import type { Binding, Device, Slot } from '../types'
import PageHeader from '../components/PageHeader.vue'
import Card from '../components/Card.vue'
import EmptyState from '../components/EmptyState.vue'
import ErrorMessage from '../components/ErrorMessage.vue'
import Toast from '../components/Toast.vue'

const route = useRoute()
const slots = ref<Slot[]>([])
const bindings = ref<Binding[]>([])
const devices = ref<Device[]>([])
const selectedSlot = ref('')
const selectedDevice = ref<Device>()
const rendered = ref('')
const toast = ref('')
const error = ref('')
const saving = ref(false)
const form = reactive({ label: '', username: '', stream_name: 'stream2' })
const assignedDeviceIds = computed(() => new Set(bindings.value.map((binding) => binding.device_id)))
const availableDevices = computed(() => devices.value.filter((device) => !assignedDeviceIds.value.has(device.id) || selectedDevice.value?.id === device.id))
const selectedSlotLabel = computed(() => slots.value.find((slot) => slot.id === selectedSlot.value)?.label || 'Kamera')

function bindingFor(slotId: string) {
  return bindings.value.find((binding) => binding.slot_id === slotId)
}

function selectDevice(device: Device) {
  selectedDevice.value = device
}

function chooseSlot(slotId: string) {
  selectedSlot.value = slotId
  const existing = bindingFor(slotId)
  form.label = existing?.label || slots.value.find((slot) => slot.id === slotId)?.label || ''
  form.username = existing?.username || ''
  form.stream_name = existing?.stream_name || 'stream2'
  if (existing?.device) selectedDevice.value = existing.device
}

function slotClass(slotId: string) {
  return {
    filled: Boolean(bindingFor(slotId)),
    selected: selectedSlot.value === slotId,
    large: slotId === 'cam5'
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

function deviceSignal(device: Device) {
  const info = raw(device)
  if (info.rtsp_port_open) return 'RTSP erreichbar'
  if (info.onvif_port_open) return 'ONVIF-Kandidat'
  return 'prüfen'
}

async function load() {
  const status = await api.status()
  slots.value = status.slots
  bindings.value = status.bindings
  devices.value = status.devices
  const deviceId = String(route.query.device || '')
  if (deviceId) selectedDevice.value = devices.value.find((device) => device.id === deviceId)
}

async function save() {
  if (!selectedDevice.value || !selectedSlot.value) return
  saving.value = true
  error.value = ''
  try {
    await api.saveBinding({
      slot_id: selectedSlot.value,
      device_id: selectedDevice.value.id,
      label: form.label,
      username: form.username,
      stream_name: form.stream_name,
      enabled: true
    })
    toast.value = 'Zuordnung gespeichert'
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Zuordnung konnte nicht gespeichert werden.'
  } finally {
    saving.value = false
  }
}

async function remove() {
  if (!selectedSlot.value) return
  await api.removeBinding(selectedSlot.value)
  toast.value = 'Zuordnung entfernt'
  await load()
}

async function render() {
  try {
    const result = await api.renderGo2RTC()
    rendered.value = result.redacted_yaml
    toast.value = `${result.rendered_streams} Stream(s) erzeugt`
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Konfiguration konnte nicht erzeugt werden.'
  }
}

async function restart() {
  try {
    await api.restartGo2RTC()
    toast.value = 'go2rtc wurde neu gestartet'
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'go2rtc konnte nicht neu gestartet werden.'
  }
}

onMounted(load)
</script>
