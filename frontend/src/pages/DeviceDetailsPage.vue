<template>
  <header class="topline">
    <div>
      <div class="eyebrow">
        <RouterLink to="/einrichtung" style="border-bottom: 1px solid var(--hairline-strong);">← Einrichtung</RouterLink>
        &nbsp;·&nbsp; Kamera-Diagnose
      </div>
      <h1 class="headline">{{ title }}</h1>
    </div>
    <div class="meta">
      <div>IP · <b>{{ device?.last_ip || '—' }}</b></div>
      <div>MAC · <b>{{ device?.mac_address || '—' }}</b></div>
    </div>
  </header>

  <div v-if="error" class="notice err"><span class="tag">FEHLER</span>{{ error }}</div>

  <div v-if="loading" class="empty">Wird geladen…</div>

  <template v-else-if="device">
    <div class="split">
      <section class="panel">
        <div class="panel-head">
          <h2>Identität</h2>
          <div class="right">
            <span class="sig" :class="{ on: raw.rtsp_port_open }">RTSP</span>
            <span class="sig" :class="{ on: raw.onvif_port_open }" style="margin-left: 4px;">ONVIF</span>
            <span class="sig" :class="{ on: raw.http_signature }" style="margin-left: 4px;">HTTP</span>
          </div>
        </div>
        <dl class="spec">
          <div><dt>IP</dt><dd>{{ device.last_ip || '—' }}</dd></div>
          <div><dt>MAC</dt><dd>{{ device.mac_address || '—' }}</dd></div>
          <div><dt>Hersteller</dt><dd>{{ device.manufacturer || '—' }}</dd></div>
          <div><dt>Modell</dt><dd>{{ device.model || '—' }}</dd></div>
          <div><dt>Seriennummer</dt><dd>{{ device.serial_number || '—' }}</dd></div>
          <div><dt>Hostname</dt><dd>{{ device.hostname || '—' }}</dd></div>
          <div><dt>Geräte-ID</dt><dd style="font-size: 11px; color: var(--ink-mute);">{{ device.id }}</dd></div>
        </dl>
      </section>

      <section class="panel">
        <div class="panel-head">
          <h2>Zugang testen</h2>
          <div class="right">
            {{ credentials?.password_set ? 'Passwort gespeichert' : 'kein Passwort' }}
          </div>
        </div>
        <div class="field">
          <span class="lbl">Benutzername</span>
          <input v-model="username" placeholder="tapo_hof" />
        </div>
        <div class="field">
          <span class="lbl">Passwort</span>
          <input v-model="password" type="password" :placeholder="credentials?.password_set ? '••••••••••••' : 'Kamera-Passwort'" />
        </div>
        <div class="field">
          <span class="lbl">Stream</span>
          <select v-model="stream">
            <option value="stream2">stream2 · empfohlen</option>
            <option value="stream1">stream1</option>
          </select>
        </div>
        <div class="btn-row">
          <button class="btn primary" :disabled="busy || !username" @click="saveCredentials">Zugang speichern</button>
          <button class="btn" :disabled="busy" @click="probe">RTSP prüfen</button>
          <button class="btn" :disabled="busy || !username || (!password && !credentials?.password_set)" @click="capture(false)">Bild testen</button>
        </div>
        <div v-if="probeResult" class="notice" :class="probeResult.success ? 'ok' : 'err'">
          <span class="tag">{{ probeResult.success ? 'OK' : 'ERR' }}</span>
          <div>
            <div>{{ probeResult.message }}</div>
            <div class="mono-mute" style="margin-top: 4px; font-size: 11px;">{{ probeResult.url_redacted }}</div>
          </div>
        </div>
      </section>
    </div>

    <section class="panel">
      <div class="panel-head">
        <h2>Referenzbild</h2>
        <div class="right">Hilft beim späteren Wiedererkennen</div>
      </div>
      <div class="btn-row">
        <button class="btn primary" :disabled="busy || !username || (!password && !credentials?.password_set)" @click="capture(true)">Bild aktualisieren</button>
        <button class="btn" :disabled="busy || !username || (!password && !credentials?.password_set)" @click="capture(false)">Nur anzeigen</button>
      </div>
      <div v-if="frame" style="display: grid; gap: 10px;">
        <img
          class="preview-frame"
          :src="`data:${frame.content_type};base64,${frame.image_base64}`"
          alt="Kamera-Referenzbild"
          style="display: block; max-width: 720px; width: 100%; border: 1px solid var(--hairline); border-radius: 4px;"
        />
        <div class="mono-mute" style="font-size: 11px;">
          Frame-ID · {{ frame.sha256.slice(0, 24) }}<span v-if="frame.saved_path"> · gespeichert unter {{ frame.saved_path }}</span>
        </div>
      </div>
      <div v-else-if="device && !referenceMissing" style="display: grid; gap: 10px;">
        <img
          class="preview-frame"
          :src="referenceImageUrl"
          alt="Gespeichertes Kamera-Referenzbild"
          style="display: block; max-width: 720px; width: 100%; border: 1px solid var(--hairline); border-radius: 4px;"
          @error="referenceMissing = true"
        />
        <div class="mono-mute" style="font-size: 11px;">Gespeichertes Referenzbild</div>
      </div>
      <div v-else class="empty">Noch kein Referenzbild hinterlegt.</div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>Rohdaten · Diagnose</h2>
      </div>
      <pre class="code">{{ JSON.stringify(raw, null, 2) }}</pre>
    </section>
  </template>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import type { Device, DeviceCredentials, FrameResult, ProbeResult } from '../types'

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
const credentials = ref<DeviceCredentials>()
const referenceRevision = ref(Date.now())
const referenceMissing = ref(false)

const title = computed(() => `${device.value?.manufacturer || 'Unbekannte'} ${device.value?.model || 'Kamera'}`.trim())
const referenceImageUrl = computed(() => device.value ? api.referenceImageUrl(device.value.id, referenceRevision.value) : '')
const raw = computed(() => {
  const v = device.value?.raw_json
  if (!v) return {} as Record<string, unknown>
  if (typeof v === 'string') {
    try { return JSON.parse(v) as Record<string, unknown> } catch { return {} as Record<string, unknown> }
  }
  return v
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

async function saveCredentials() {
  if (!device.value) return
  busy.value = true
  error.value = ''
  try {
    credentials.value = await api.saveDeviceCredentials(device.value.id, { username: username.value, password: password.value, stream: stream.value })
    password.value = ''
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Zugangsdaten konnten nicht gespeichert werden.'
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
    if (save) {
      referenceMissing.value = false
      referenceRevision.value = Date.now()
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Bild konnte nicht gezogen werden.'
  } finally {
    busy.value = false
  }
}

onMounted(async () => {
  try {
    device.value = await api.device(String(route.params.id))
    credentials.value = await api.deviceCredentials(device.value.id)
    username.value = credentials.value.username || ''
    stream.value = credentials.value.stream || 'stream2'
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Kamera konnte nicht geladen werden.'
  } finally {
    loading.value = false
  }
})
</script>
