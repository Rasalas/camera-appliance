<template>
  <PageHeader title="Gerät zuordnen" subtitle="Kamera einem festen Anzeigeplatz zuweisen" />
  <ErrorMessage :message="error" />
  <div class="grid two">
    <Card>
      <h2>Gerät</h2>
      <select v-model="form.device_id">
        <option value="">Gerät auswählen</option>
        <option v-for="device in devices" :key="device.id" :value="device.id">
          {{ device.manufacturer || 'Unbekannt' }} {{ device.model || 'RTSP-Gerät' }} - {{ device.last_ip || 'ohne IP' }}
        </option>
      </select>
      <div v-if="selectedDevice" class="detail-box">
        <p>IP: {{ selectedDevice.last_ip || 'unbekannt' }}</p>
        <p>MAC: {{ selectedDevice.mac_address || 'unbekannt' }}</p>
        <p>Seriennummer: {{ selectedDevice.serial_number || 'unbekannt' }}</p>
      </div>
    </Card>
    <Card>
      <h2>Zuordnen als</h2>
      <SlotSelector v-model="form.slot_id" :slots="slots" />
      <label>Anzeigename<input v-model="form.label" placeholder="Hof" /></label>
      <label>Benutzername<input v-model="form.username" placeholder="tapo_hof" /></label>
      <label>Stream
        <select v-model="form.stream_name">
          <option value="stream2">stream2</option>
          <option value="stream1">stream1</option>
        </select>
      </label>
      <button class="action-button primary" :disabled="!form.slot_id || !form.device_id || saving" @click="save">Zuordnung speichern</button>
    </Card>
  </div>
  <Toast :message="toast" />
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import type { Device, Slot } from '../types'
import PageHeader from '../components/PageHeader.vue'
import Card from '../components/Card.vue'
import SlotSelector from '../components/SlotSelector.vue'
import ErrorMessage from '../components/ErrorMessage.vue'
import Toast from '../components/Toast.vue'

const route = useRoute()
const router = useRouter()
const devices = ref<Device[]>([])
const slots = ref<Slot[]>([])
const saving = ref(false)
const error = ref('')
const toast = ref('')
const form = reactive({ slot_id: '', device_id: String(route.params.deviceId || ''), label: '', username: '', stream_name: 'stream2' })
const selectedDevice = computed(() => devices.value.find((device) => device.id === form.device_id))

async function save() {
  saving.value = true
  error.value = ''
  try {
    await api.saveBinding(form)
    toast.value = 'Zuordnung gespeichert'
    await router.push('/bindings')
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Zuordnung konnte nicht gespeichert werden.'
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  const status = await api.status()
  devices.value = status.devices
  slots.value = status.slots
})
</script>
