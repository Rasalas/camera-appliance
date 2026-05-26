<template>
  <PageHeader title="Kamera" subtitle="Details und Bearbeitung dieser Kamera-Ressource">
    <RouterLink v-if="device" class="button-link secondary" :to="`/display?camera=${device.id}`">In Anzeige verwenden</RouterLink>
  </PageHeader>
  <ErrorMessage :message="error" />
  <LoadingState v-if="loading" />
  <template v-else-if="device">
    <div class="resource-sections">
      <Card>
        <h2>Identität</h2>
        <p>{{ title }}</p>
        <dl class="detail-list">
          <div><dt>IP</dt><dd>{{ device.last_ip || 'unbekannt' }}</dd></div>
          <div><dt>MAC</dt><dd>{{ device.mac_address || 'unbekannt' }}</dd></div>
          <div><dt>Hersteller</dt><dd>{{ device.manufacturer || 'unbekannt' }}</dd></div>
          <div><dt>Modell</dt><dd>{{ device.model || 'unbekannt' }}</dd></div>
          <div><dt>Hostname</dt><dd>{{ device.hostname || 'unbekannt' }}</dd></div>
          <div><dt>Geräte-ID</dt><dd class="mono">{{ device.id }}</dd></div>
        </dl>
        <div class="signal-grid">
          <span :class="raw.rtsp_port_open ? 'signal ok' : 'signal muted'">RTSP 554</span>
          <span :class="raw.onvif_port_open ? 'signal ok' : 'signal muted'">ONVIF 2020</span>
          <span :class="raw.http_signature ? 'signal ok' : 'signal muted'">HTTP-Signal</span>
        </div>
      </Card>

      <Card>
        <h2>Zugang und Stream</h2>
        <label>Benutzername<input v-model="username" placeholder="tapo_hof" /></label>
        <label>Passwort<input v-model="password" type="password" placeholder="Kamera-Passwort" /></label>
        <label>Stream
          <select v-model="stream">
            <option value="stream2">stream2</option>
            <option value="stream1">stream1</option>
          </select>
        </label>
        <div class="button-row">
          <button class="action-button secondary" :disabled="busy" @click="probe">Zugang prüfen</button>
          <button class="action-button primary" :disabled="busy || !username || !password" @click="capture(false)">Frame testen</button>
        </div>
        <p v-if="probeResult">{{ probeResult.message }}</p>
        <p v-if="probeResult" class="mono">{{ probeResult.url_redacted }}</p>
      </Card>
    </div>

    <Card>
      <h2>Referenzbild</h2>
      <p>Ein gespeichertes Bild gehört zur Kamera-Ressource und hilft später bei der visuellen Identifikation.</p>
      <div class="button-row">
        <button class="action-button primary" :disabled="busy || !username || !password" @click="capture(true)">Frame ziehen und hinterlegen</button>
        <button class="action-button secondary" :disabled="busy || !username || !password" @click="capture(false)">Nur anzeigen</button>
      </div>
      <img v-if="frame" class="preview-frame" :src="`data:${frame.content_type};base64,${frame.image_base64}`" alt="Kamera-Referenzbild" />
      <p v-if="frame" class="mono">Frame-ID: {{ frame.sha256.slice(0, 24) }}</p>
      <p v-if="frame?.saved_path">Gespeichert unter {{ frame.saved_path }}</p>
    </Card>

    <Card>
      <h2>Diagnose</h2>
      <pre>{{ JSON.stringify(raw, null, 2) }}</pre>
    </Card>
  </template>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import type { Device, FrameResult, ProbeResult } from '../types'
import PageHeader from '../components/PageHeader.vue'
import Card from '../components/Card.vue'
import ErrorMessage from '../components/ErrorMessage.vue'
import LoadingState from '../components/LoadingState.vue'

const route = useRoute()
const device = ref<Device>()
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const username = ref('')
const password = ref('')
const stream = ref('stream2')
const probeResult = ref<ProbeResult>()
const frame = ref<FrameResult>()
const title = computed(() => `${device.value?.manufacturer || 'Unbekannte'} ${device.value?.model || 'Kamera'}`.trim())
const raw = computed(() => {
  const value = device.value?.raw_json
  if (!value) return {} as Record<string, unknown>
  if (typeof value === 'string') {
    try { return JSON.parse(value) as Record<string, unknown> } catch { return {} as Record<string, unknown> }
  }
  return value
})

async function probe() {
  if (!device.value) return
  busy.value = true
  error.value = ''
  try {
    probeResult.value = await api.probeDevice(device.value.id, { username: username.value, password: password.value, stream: stream.value })
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Zugang konnte nicht geprüft werden.'
  } finally {
    busy.value = false
  }
}

async function capture(save: boolean) {
  if (!device.value) return
  busy.value = true
  error.value = ''
  try {
    frame.value = await api.captureFrame(device.value.id, { username: username.value, password: password.value, stream: stream.value, save })
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Vorschaubild konnte nicht gezogen werden.'
  } finally {
    busy.value = false
  }
}

onMounted(async () => {
  try {
    device.value = await api.device(String(route.params.id))
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Kamera konnte nicht geladen werden.'
  } finally {
    loading.value = false
  }
})
</script>
