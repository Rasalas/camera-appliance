<template>
  <PageHeader title="Anzeige" subtitle="Welche Kamera auf welchem Anzeigeplatz erscheint" />
  <ErrorMessage :message="error" />

  <div class="display-editor">
    <Card class="layout-card">
      <h2>Anzeige-Layout</h2>
      <p>Klicke einen Platz an, um seine Kamera und Beschriftung zu bearbeiten.</p>
      <div class="camera-layout">
        <button v-for="slot in slots" :key="slot.id" class="layout-slot" :class="slotClass(slot.id)" @click="selectSlot(slot.id)">
          <span class="slot-id">{{ slot.id }}</span>
          <strong>{{ bindingFor(slot.id)?.label || slot.label }}</strong>
          <small>{{ bindingFor(slot.id)?.device?.last_ip || 'nicht zugeordnet' }}</small>
        </button>
      </div>
    </Card>

    <Card>
      <h2>{{ selectedSlot ? `${selectedSlot} bearbeiten` : 'Platz bearbeiten' }}</h2>
      <EmptyState v-if="!selectedSlot" text="Wähle links einen Anzeigeplatz." />
      <template v-else>
        <label>Kamera
          <select v-model="form.device_id">
            <option value="">Keine Kamera</option>
            <option v-for="device in devices" :key="device.id" :value="device.id">
              {{ deviceTitle(device) }} · {{ device.last_ip || 'ohne IP' }}
            </option>
          </select>
        </label>
        <label>Anzeigename<input v-model="form.label" :placeholder="slotLabel" /></label>
        <label>Benutzername<input v-model="form.username" placeholder="tapo_hof" /></label>
        <label>Stream
          <select v-model="form.stream_name">
            <option value="stream2">stream2</option>
            <option value="stream1">stream1</option>
          </select>
        </label>
        <div class="button-row">
          <button class="action-button primary" :disabled="!form.device_id || saving" @click="save">Speichern</button>
          <button class="action-button danger" :disabled="!bindingFor(selectedSlot)" @click="remove">Entfernen</button>
          <RouterLink v-if="form.device_id" class="button-link secondary" :to="`/cameras/${form.device_id}`">Kamera öffnen</RouterLink>
        </div>
      </template>
    </Card>
  </div>

  <Card>
    <div class="card-row">
      <div>
        <h2>Ausspielen</h2>
        <p>Erzeugt die go2rtc-Konfiguration aus der Anzeige-Zuordnung.</p>
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
const devices = ref<Device[]>([])
const bindings = ref<Binding[]>([])
const selectedSlot = ref('')
const rendered = ref('')
const toast = ref('')
const error = ref('')
const saving = ref(false)
const form = reactive({ device_id: '', label: '', username: '', stream_name: 'stream2' })
const slotLabel = computed(() => slots.value.find((slot) => slot.id === selectedSlot.value)?.label || 'Kamera')

function bindingFor(slotId: string) {
  return bindings.value.find((binding) => binding.slot_id === slotId)
}

function slotClass(slotId: string) {
  return {
    filled: Boolean(bindingFor(slotId)),
    selected: selectedSlot.value === slotId,
    large: slotId === 'cam5'
  }
}

function selectSlot(slotId: string) {
  selectedSlot.value = slotId
  const binding = bindingFor(slotId)
  form.device_id = binding?.device_id || String(route.query.camera || '')
  form.label = binding?.label || slots.value.find((slot) => slot.id === slotId)?.label || ''
  form.username = binding?.username || ''
  form.stream_name = binding?.stream_name || 'stream2'
}

function deviceTitle(device: Device) {
  return `${device.manufacturer || 'Unbekannte'} ${device.model || 'Kamera'}`.trim()
}

async function load() {
  const status = await api.status()
  slots.value = status.slots
  devices.value = status.devices
  bindings.value = status.bindings
  if (!selectedSlot.value && slots.value.length) selectSlot(slots.value[0].id)
}

async function save() {
  if (!selectedSlot.value || !form.device_id) return
  saving.value = true
  try {
    await api.saveBinding({ slot_id: selectedSlot.value, device_id: form.device_id, label: form.label, username: form.username, stream_name: form.stream_name, enabled: true })
    toast.value = 'Anzeigeplatz gespeichert'
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Anzeigeplatz konnte nicht gespeichert werden.'
  } finally {
    saving.value = false
  }
}

async function remove() {
  if (!selectedSlot.value) return
  await api.removeBinding(selectedSlot.value)
  toast.value = 'Anzeigeplatz geleert'
  await load()
}

async function render() {
  const result = await api.renderGo2RTC()
  rendered.value = result.redacted_yaml
  toast.value = `${result.rendered_streams} Stream(s) erzeugt`
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
