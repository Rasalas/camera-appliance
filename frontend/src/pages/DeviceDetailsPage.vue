<template>
  <header class="topline">
    <div>
      <nav aria-label="Zurück"><RouterLink to="/einrichtung" class="mono-mute">← Kameras</RouterLink></nav>
      <h1 class="headline">{{ title }}{{ editing ? ' bearbeiten' : uploadEditing ? ' · Bild-Upload' : '' }}</h1>
    </div>
    <div v-if="device && !editing && !uploadEditing" class="meta sig-row">
      <span class="sig" :class="{ on: raw.rtsp_port_open }">RTSP</span>
      <span class="sig" :class="{ on: raw.onvif_port_open }">ONVIF</span>
      <span class="sig" :class="{ on: raw.http_signature }">HTTP</span>
    </div>
  </header>

  <div v-if="error" class="notice err"><span class="tag">FEHLER</span>{{ error }}</div>

  <div v-if="loading" class="empty">Wird geladen…</div>

  <template v-else-if="device">
    <div v-if="!editing && !uploadEditing" class="btn-row">
      <button class="btn" :class="{ live: shown }" type="button" :disabled="busyShow" @click="toggleShow">
        {{ busyShow ? '…' : shown ? '✓ Sichtbar in der Ansicht' : 'In der Ansicht anzeigen' }}
      </button>
      <RouterLink v-if="shown" class="btn ghost" to="/">Zur Live-Ansicht</RouterLink>
      <div class="spacer" />
      <span class="mono-mute" style="font-size: 11px;">{{ shown ? 'Position wird in der Live-Ansicht festgelegt.' : 'Anzeigen, dann in der Ansicht platzieren.' }}</span>
    </div>

    <div v-if="editing" class="editor-actions"><button class="btn primary" :disabled="busy || savingAll || !editDirty" @click="saveAll">{{ savingAll ? 'Speichert…' : 'Kamera speichern' }}</button><RouterLink class="btn" :to="`/kamera/${device.id}`">Abbrechen</RouterLink><span role="status">{{ editDirty ? 'Ungespeicherte Änderungen' : 'Gespeichert' }}</span></div>
    <RouterLink v-if="uploadEditing" class="btn ghost" :to="`/kamera/${device.id}`">← Zur Kameradetailseite</RouterLink>
    <div v-if="!uploadEditing" class="device-sections">
      <section v-if="!editing" class="panel">
        <div class="panel-head">
          <h2>Identität</h2>
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

      <section v-if="editing" id="zugang" :inert="savingAll" class="panel edit-section">
        <div class="panel-head">
          <h2>Kamera-Zugang</h2>
          <div class="right">
            {{ credentials?.password_set ? 'Kamera-Passwort' : identitySummary }}
          </div>
        </div>
        <div class="field">
          <span class="lbl">Benutzername</span>
          <input aria-label="Benutzername" v-model="username" placeholder="tapo_hof" />
        </div>
        <div class="field">
          <span class="lbl">Passwort</span>
          <input aria-label="Passwort" v-model="password" type="password" :placeholder="credentials?.password_set ? '••••••••••••' : 'Kamera-Passwort'" />
        </div>
        <div class="field">
          <span class="lbl">Stream</span>
          <select aria-label="Stream" v-model="stream">
            <option value="stream2">stream2 · empfohlen</option>
            <option value="stream1">stream1</option>
          </select>
        </div>
        <div class="btn-row">
          <button class="btn" :disabled="busy" @click="probe">RTSP prüfen</button>
          <button class="btn" :disabled="busy || !canCapture" @click="capture(false)">Bild testen</button>
        </div>
        <div v-if="!credentials?.password_set && credentialIdentities.length" class="mono-mute" style="font-size: 11px;">
          Kein Kamera-Passwort gesetzt. Beim Bildtest werden {{ credentialIdentities.length }} gespeicherte Identität(en) ausprobiert.
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

    <section v-if="editing && relayIds.length" id="verbindung" :inert="savingAll" class="panel edit-section">
      <div class="panel-head">
        <h2>Verbindung</h2>
        <div class="right">aktiv über {{ activePathLabel }}</div>
      </div>
      <div class="split">
        <div class="field">
          <span class="lbl">Verbindungsweg</span>
          <select aria-label="Verbindungsweg" v-model="pathPolicy">
            <option value="auto">Automatisch (empfohlen)</option>
            <option value="relay_only">Muss über Relay</option>
            <option value="direct_only">Nur direkt</option>
            <option v-if="pathPolicy === 'prefer_direct'" value="prefer_direct">Direkt bevorzugen (alt)</option>
            <option v-if="pathPolicy === 'prefer_relay'" value="prefer_relay">Relay bevorzugen (alt)</option>
          </select>
        </div>
        <div v-if="pathPolicy !== 'direct_only'" class="relay-endpoints" style="align-content: center;">
          <div v-for="relayId in relayIds" :key="relayId" class="endpoint-summary">
            <span class="endpoint-state" :class="endpointStateClass(relayId)">{{ endpointStateLabel(relayId) }}</span>
            <span class="mono-mute">{{ relayNameFor(relayId) }} · Port {{ endpointPort(relayId) }}</span>
          </div>
        </div>
      </div>
      <details class="advanced">
        <summary>Feinjustage</summary>
        <div class="relay-endpoints" style="margin-top: 12px;">
          <div v-for="relayId in relayIds" :key="`adv-${relayId}`" class="relay-endpoint-row">
            <span>{{ relayNameFor(relayId) }}</span>
            <input v-model="settings[endpointKey(relayId, 'port')]" class="compact-input" :placeholder="`Port · auto ${autoPortFor(relayId)}`" />
            <input v-model="settings[endpointKey(relayId, 'host')]" class="compact-input" :placeholder="relayHostFor(relayId) || 'go2rtc-Host'" />
            <input v-model="settings[endpointKey(relayId, 'target_host')]" class="compact-input" :placeholder="device.last_ip || 'Ziel-IP'" />
            <input v-model="settings[endpointKey(relayId, 'target_port')]" class="compact-input" placeholder="554" />
          </div>
        </div>
        <div class="btn-row" style="margin-top: 12px;">
          <span class="mono-mute" style="font-size: 11px;">Leere Felder = automatisch (Port aus Kameraplatz, Ziel = Kamera-IP).</span>
        </div>
      </details>
    </section>

    <section v-if="editing" id="anzeige" :inert="savingAll" class="panel edit-section">
      <div class="panel-head">
        <h2>Bild &amp; Anzeige</h2>
        <div class="right">{{ displaySummary }}</div>
      </div>

      <div class="split-3-2">
        <div class="transform-preview">
          <div
            v-if="previewImageSrc"
            class="transform-preview-stage"
            :class="{ grabbable: canPan }"
            @pointerdown="startStagePan"
            @wheel.prevent="onStageWheel"
          >
            <img
              class="transform-preview-image"
              :src="previewImageSrc"
              alt="Transformierte Kameravorschau"
              :style="displayPreviewStyle"
            />
            <div class="stage-tools" @pointerdown.stop @wheel.stop>
              <button class="btn icon sm" type="button" title="90° drehen" @click="rotateStage">⟳</button>
              <button class="btn sm" :class="{ live: mirror }" type="button" title="Horizontal spiegeln" @click="toggleMirror">⇋</button>
              <button class="btn sm" :class="{ live: flip }" type="button" title="Vertikal spiegeln" @click="toggleFlip">⇅</button>
              <button class="btn sm" type="button" :title="fitMode === 'cover' ? 'Ganzes Bild zeigen' : 'Format füllen'" @click="toggleFit">{{ fitMode === 'cover' ? 'Füllen' : 'Ganz' }}</button>
              <button class="btn icon sm" type="button" title="Hineinzoomen" @click="zoomStage(-1)">＋</button>
              <button class="btn icon sm" type="button" title="Herauszoomen" @click="zoomStage(1)">－</button>
              <button class="btn icon sm" type="button" title="Zuschnitt zurücksetzen" @click="resetDisplay">⟲</button>
            </div>
          </div>
          <div v-else class="empty">Noch kein Referenzbild — zuerst Zugang speichern, dann rechts „Referenzbild aufnehmen“.</div>
          <div v-if="previewImageSrc" class="mono-mute" style="margin-top: 8px; font-size: 11px;">
            <template v-if="frame">Frame-ID · {{ frame.sha256.slice(0, 24) }}<span v-if="frame.credential_source"> · Zugang: {{ frame.credential_source }}</span></template>
            <template v-else>Gespeichertes Referenzbild</template>
            · Ziehen = Ausschnitt verschieben · Mausrad = Zoom · Änderungen werden mit „Kamera speichern“ übernommen
          </div>
        </div>

        <div class="display-controls">
          <div class="btn-row">
            <button class="btn primary" :disabled="busy || !canCapture" @click="capture(true)">{{ previewImageSrc ? 'Referenzbild aktualisieren' : 'Referenzbild aufnehmen' }}</button>
          </div>
          <div class="mono-mute" style="font-size: 11px;">Das Referenzbild dient als Vorschau für den Zuschnitt und hilft beim späteren Wiedererkennen der Kamera.</div>

          <details class="advanced">
            <summary>Feinjustage</summary>
            <div class="crop-grid" style="margin-top: 12px;">
              <div class="field"><span class="lbl">Crop X</span><input aria-label="Crop X" v-model.number="cropX" type="number" min="0" max="99" /></div>
              <div class="field"><span class="lbl">Crop Y</span><input aria-label="Crop Y" v-model.number="cropY" type="number" min="0" max="99" /></div>
              <div class="field"><span class="lbl">Breite %</span><input aria-label="Breite %" v-model.number="cropWidth" type="number" min="1" max="100" /></div>
              <div class="field"><span class="lbl">Höhe %</span><input aria-label="Höhe %" v-model.number="cropHeight" type="number" min="1" max="100" /></div>
            </div>
          </details>

          <div class="btn-row">
            <button class="btn" :disabled="busy" @click="resetDisplay">Zurücksetzen</button>
            <button class="btn ghost" :disabled="busy || editDirty" title="Verwendet die gespeicherten Kamera-Einstellungen" @click="renderAfterDisplay">go2rtc erzeugen</button>
          </div>
        </div>
      </div>
    </section>

    <template v-if="!editing && !uploadEditing">
      <section class="panel edit-section"><div class="panel-head"><h2>Kamera-Zugang</h2><RouterLink class="btn ghost desktop-primary" :to="`/kamera/${device.id}/bearbeiten#zugang`">Bearbeiten</RouterLink></div><dl class="spec"><div><dt>Benutzername</dt><dd>{{ username || 'Gemeinsame Identität' }}</dd></div><div><dt>Passwort</dt><dd>{{ credentials?.password_set ? 'Kameraspezifisch gespeichert' : identitySummary }}</dd></div><div><dt>Stream</dt><dd>{{ stream }}</dd></div></dl></section>
      <section class="panel edit-section"><div class="panel-head"><h2>Verbindung und Anzeige</h2><RouterLink class="btn ghost desktop-primary" :to="`/kamera/${device.id}/bearbeiten#anzeige`">Bearbeiten</RouterLink></div><dl class="spec"><div><dt>Verbindung</dt><dd>{{ activePathLabel }}</dd></div><div><dt>Ausrichtung</dt><dd>{{ displaySummary }}</dd></div></dl><img v-if="previewImageSrc" :src="previewImageSrc" alt="Gespeichertes Referenzbild" class="device-reference" /></section>
      <section class="panel edit-section"><div class="panel-head"><h2>Bild-Upload</h2><RouterLink class="btn ghost" :to="`/kamera/${device.id}/bild-upload`">Bearbeiten</RouterLink></div><p class="mono-mute">Bildausschnitt, Privatbereiche, Zeitangabe, Dateiname und Zeitplan für diese Kamera.</p></section>
      <RouterLink class="mobile-fab" aria-label="Kamera bearbeiten" :to="`/kamera/${device.id}/bearbeiten`"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m4 16 12-12 4 4-12 12H4zM14 6l4 4"/></svg></RouterLink>
    </template>
    <SnapshotUpload v-if="uploadEditing" :key="device.id" :device-id="device.id" :camera-label="title" :image-src="previewImageSrc" :username="username" :password="password" :stream="stream" :can-capture="canCapture" :camera-busy="busy" />

    <section v-if="!editing && !uploadEditing" class="panel">
      <details class="advanced">
        <summary>Rohdaten · Diagnose</summary>
        <pre class="code" style="margin-top: 10px;">{{ JSON.stringify(raw, null, 2) }}</pre>
      </details>
    </section>
  </template>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import { useDraftGuard } from '../composables/discardChanges'
import SnapshotUpload from '../components/SnapshotUpload.vue'
import type { Binding, CredentialIdentity, Device, DeviceCredentials, FrameResult, ProbeResult, RelayStatus, Slot } from '../types'

const route = useRoute(), router = useRouter()
const editing = computed(() => route.name === 'device-edit')
const uploadEditing = computed(() => route.name === 'device-upload')
const editBaseline = ref(''), credentialBaseline = ref(''), savingAll = ref(false)
const editSnapshot = computed(() => JSON.stringify({username:username.value,password:password.value,stream:stream.value,path:pathPolicy.value,rotation:rotation.value,mirror:mirror.value,flip:flip.value,fit:fitMode.value,crop:[cropX.value,cropY.value,cropWidth.value,cropHeight.value],endpoints:Object.fromEntries(Object.entries(settings.value).filter(([key])=>key.startsWith('camera.relay_endpoint.')))}))
const editDirty = computed(() => ready.value && editing.value && editSnapshot.value !== editBaseline.value)
useDraftGuard(() => editDirty.value, () => { editBaseline.value = editSnapshot.value })
async function saveAll() {
  if (savingAll.value || !device.value) return
  savingAll.value = true
  try {
    if (JSON.stringify({ username: username.value, password: password.value }) !== credentialBaseline.value) {
      await saveCredentials()
      if(error.value)return
      credentialBaseline.value=JSON.stringify({username:username.value,password:password.value})
    }
    await saveDisplay()
    if(error.value)return
    if(relayIds.value.length) await saveEndpointOverrides()
    if(error.value)return
    editBaseline.value = editSnapshot.value
    await router.push(`/kamera/${device.value.id}`)
  } finally { savingAll.value = false }
}
const device = ref<Device>()
const slots = ref<Slot[]>([])
const busyShow = ref(false)
const ready = ref(false)
let stopStagePan: (() => void) | null = null
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const username = ref('')
const password = ref('')
const stream = ref('stream2')
const probeResult = ref<ProbeResult>()
const frame = ref<FrameResult>()
const credentials = ref<DeviceCredentials>()
const credentialIdentities = ref<CredentialIdentity[]>([])
const bindings = ref<Binding[]>([])
const settings = ref<Record<string, string>>({})
const referenceRevision = ref(Date.now())
const referenceMissing = ref(false)
const rotation = ref(0)
const mirror = ref(false)
const flip = ref(false)
const fitMode = ref<'cover' | 'contain'>('contain')
const cropX = ref(0)
const cropY = ref(0)
const cropWidth = ref(100)
const cropHeight = ref(100)
const pathPolicy = ref('auto')
const relayStatuses = ref<RelayStatus[]>([])

// Mirrors the backend's auto port scheme (paths.go): relay n → base 18554+20n, slot m → +m-1.
const relayPortBaseDefault = 18554
const relayPortBaseSpacing = 20

const title = computed(() => `${device.value?.manufacturer || 'Unbekannte'} ${device.value?.model || 'Kamera'}`.trim())
const relayIds = computed(() =>
  (settings.value['camera.relay.ids'] || '')
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean)
)
const activePathLabel = computed(() => {
  const deviceID = device.value?.id
  if (!deviceID) return '—'
  const kind = settings.value[`camera.active_path.${deviceID}.kind`]
  if (kind === 'relay') {
    const relayId = settings.value[`camera.active_path.${deviceID}.relay_id`]
    return relayId && relayId !== 'manual' ? `Relay ${relayNameFor(relayId)}` : 'Relay'
  }
  return kind === 'direct' ? 'Direkt' : '—'
})
const shown = computed(() => !!device.value && bindings.value.some((b) => b.device_id === device.value!.id))
const canPan = computed(() => clamp(Number(cropWidth.value) || 100, 1, 100) < 100 || clamp(Number(cropHeight.value) || 100, 1, 100) < 100)
const referenceImageUrl = computed(() => device.value ? api.referenceImageUrl(device.value.id, referenceRevision.value) : '')
const canCapture = computed(() => Boolean((username.value && (password.value || credentials.value?.password_set)) || credentialIdentities.value.length))
const identitySummary = computed(() => credentialIdentities.value.length ? `${credentialIdentities.value.length} Identität(en)` : 'kein Passwort')
const previewImageSrc = computed(() => {
  if (frame.value) return `data:${frame.value.content_type};base64,${frame.value.image_base64}`
  return referenceMissing.value ? '' : referenceImageUrl.value
})
const displaySummary = computed(() => `${rotation.value}° · ${fitMode.value} · ${cropWidth.value}×${cropHeight.value}%`)
const displayPreviewStyle = computed(() => {
  const crop = normalizedCrop()
  const width = 10000 / crop.width
  const height = 10000 / crop.height
  const left = -(crop.x / crop.width) * 100
  const top = -(crop.y / crop.height) * 100
  const scaleX = mirror.value ? -1 : 1
  const scaleY = flip.value ? -1 : 1
  return {
    left: `${left}%`,
    top: `${top}%`,
    width: `${width}%`,
    height: `${height}%`,
    objectFit: fitMode.value,
    transform: `rotate(${rotation.value}deg) scaleX(${scaleX}) scaleY(${scaleY})`
  }
})
const raw = computed(() => {
  const v = device.value?.raw_json
  if (!v) return {} as Record<string, unknown>
  if (typeof v === 'string') {
    try { return JSON.parse(v) as Record<string, unknown> } catch { return {} as Record<string, unknown> }
  }
  return v
})

function relayNameFor(relayId: string) {
  return settings.value[`camera.relay.${relayId}.name`] || relayId
}
function relayHostFor(relayId: string) {
  return settings.value[`camera.relay.${relayId}.host`] || ''
}
function endpointKey(relayId: string, field: string) {
  return `camera.relay_endpoint.${device.value?.id}.${relayId}.${field}`
}
function myEndpoint(relayId: string) {
  return relayStatuses.value.find((relay) => relay.id === relayId)?.endpoints.find((endpoint) => endpoint.device_id === device.value?.id)
}
function endpointPort(relayId: string) {
  return myEndpoint(relayId)?.local_port || settings.value[endpointKey(relayId, 'port')] || autoPortFor(relayId)
}
function endpointStateLabel(relayId: string) {
  const state = myEndpoint(relayId)?.state
  if (state === 'ok') return 'OK'
  if (state === 'failed') return 'Offline'
  return state ? 'Unvollständig' : 'kein Status'
}
function endpointStateClass(relayId: string) {
  const state = myEndpoint(relayId)?.state
  if (state === 'ok') return 'ok'
  if (state === 'failed') return 'err'
  return 'warn'
}
function autoPortFor(relayId: string) {
  const slotID = bindings.value.find((binding) => binding.device_id === device.value?.id)?.slot_id || ''
  const slotNumber = Number(slotID.replace(/^\D+/, ''))
  if (!Number.isFinite(slotNumber) || slotNumber <= 0) return '—'
  const index = Math.max(0, relayIds.value.indexOf(relayId))
  const base = Number(settings.value[`camera.relay.${relayId}.port_base`]) || relayPortBaseDefault + relayPortBaseSpacing * index
  return String(base + slotNumber - 1)
}

async function loadRelayStatuses() {
  if (!relayIds.value.length) return
  try {
    relayStatuses.value = await api.relayStatus()
  } catch {
    relayStatuses.value = []
  }
}

async function saveEndpointOverrides() {
  if (!device.value) return
  busy.value = true
  error.value = ''
  try {
    const values: Record<string, string> = {}
    for (const relayId of relayIds.value) {
      for (const field of ['port', 'host', 'target_host', 'target_port']) {
        const key = endpointKey(relayId, field)
        if (key in settings.value) values[key] = settings.value[key]
      }
    }
    await api.saveSettings(values)
    await loadRelayStatuses()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Relay-Endpunkte konnten nicht gespeichert werden.'
  } finally {
    busy.value = false
  }
}

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

async function saveDisplay() {
  if (!device.value) return
  busy.value = true
  error.value = ''
  try {
    const deviceID = device.value.id
    const crop = normalizedCrop()
    const values: Record<string, string> = {
      [`camera.display.${deviceID}.rotation`]: String(rotation.value),
      [`camera.display.${deviceID}.mirror`]: String(mirror.value),
      [`camera.display.${deviceID}.flip`]: String(flip.value),
      [`camera.display.${deviceID}.fit_mode`]: fitMode.value,
      [`camera.display.${deviceID}.crop_x`]: String(crop.x),
      [`camera.display.${deviceID}.crop_y`]: String(crop.y),
      [`camera.display.${deviceID}.crop_width`]: String(crop.width),
      [`camera.display.${deviceID}.crop_height`]: String(crop.height),
      [`camera.path_policy.${deviceID}`]: pathPolicy.value,
      [`camera.credentials.${deviceID}.stream`]: stream.value
    }
    await api.saveSettings(values)
    const binding = bindings.value.find((item) => item.device_id === deviceID)
    if (binding && binding.stream_name !== stream.value) {
      await api.saveBinding({ ...binding, stream_name: stream.value })
      bindings.value = await api.bindings()
    }
    if (credentials.value) {
      credentials.value.stream = stream.value
    }
    Object.assign(settings.value, values)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Anzeige konnte nicht gespeichert werden.'
  } finally {
    busy.value = false
  }
}

async function renderAfterDisplay() {
  if (editDirty.value) return
  busy.value = true
  try {
    await api.renderGo2RTC()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'go2rtc konnte nicht erzeugt werden.'
  } finally {
    busy.value = false
  }
}

function resetDisplay() {
  rotation.value = 0
  mirror.value = false
  flip.value = false
  fitMode.value = 'contain'
  cropX.value = 0
  cropY.value = 0
  cropWidth.value = 100
  cropHeight.value = 100
}

async function toggleShow() {
  if (!device.value || busyShow.value) return
  busyShow.value = true
  error.value = ''
  try {
    const existing = bindings.value.find((b) => b.device_id === device.value!.id)
    if (existing) {
      await api.removeBinding(existing.slot_id)
    } else {
      const used = new Set(bindings.value.map((b) => b.slot_id))
      const free = slots.value.find((slot) => !used.has(slot.id))
      if (!free) {
        error.value = 'Maximal ' + slots.value.length + ' Kameras gleichzeitig sichtbar. Blende zuerst eine andere aus.'
        return
      }
      await api.saveBinding({
        slot_id: free.id,
        device_id: device.value.id,
        label: title.value,
        username: credentials.value?.username || username.value || '',
        stream_name: credentials.value?.stream || stream.value || free.default_stream || 'stream2',
        enabled: true
      })
    }
    await api.renderGo2RTC()
    await api.restartGo2RTC().catch(() => undefined)
    bindings.value = await api.bindings()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Sichtbarkeit konnte nicht geändert werden.'
  } finally {
    busyShow.value = false
  }
}

function rotateStage() {
  rotation.value = (rotation.value + 90) % 360
}
function toggleMirror() {
  mirror.value = !mirror.value
}
function toggleFlip() {
  flip.value = !flip.value
}
function toggleFit() {
  fitMode.value = fitMode.value === 'cover' ? 'contain' : 'cover'
}
function zoomStage(direction: number) {
  const step = 8
  const w = clamp((Number(cropWidth.value) || 100) + direction * step, 20, 100)
  const h = clamp((Number(cropHeight.value) || 100) + direction * step, 20, 100)
  const centerX = (Number(cropX.value) || 0) + (Number(cropWidth.value) || 100) / 2
  const centerY = (Number(cropY.value) || 0) + (Number(cropHeight.value) || 100) / 2
  cropWidth.value = w
  cropHeight.value = h
  cropX.value = clamp(centerX - w / 2, 0, 100 - w)
  cropY.value = clamp(centerY - h / 2, 0, 100 - h)
  fitMode.value = 'cover'
}
function onStageWheel(event: WheelEvent) {
  zoomStage(event.deltaY > 0 ? 1 : -1)
}
function startStagePan(event: PointerEvent) {
  if (!canPan.value || event.button !== 0) return
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const startX = event.clientX
  const startY = event.clientY
  const baseX = Number(cropX.value) || 0
  const baseY = Number(cropY.value) || 0
  const w = clamp(Number(cropWidth.value) || 100, 1, 100)
  const h = clamp(Number(cropHeight.value) || 100, 1, 100)
  event.preventDefault()
  const move = (moveEvent: PointerEvent) => {
    const dx = ((moveEvent.clientX - startX) / rect.width) * w
    const dy = ((moveEvent.clientY - startY) / rect.height) * h
    cropX.value = clamp(baseX - dx, 0, 100 - w)
    cropY.value = clamp(baseY - dy, 0, 100 - h)
    moveEvent.preventDefault()
  }
  const up = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    stopStagePan = null
  }
  stopStagePan = up
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
}

async function capture(save: boolean) {
  if (!device.value) return
  busy.value = true
  error.value = ''
  try {
    frame.value = await api.captureFrame(device.value.id, { username: username.value, password: password.value, stream: stream.value, save })
    credentials.value = await api.deviceCredentials(device.value.id)
    if (!username.value && credentials.value.username) username.value = credentials.value.username
    if (credentials.value.stream) stream.value = credentials.value.stream
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

function loadDisplaySettings() {
  if (!device.value) return
  const deviceID = device.value.id
  rotation.value = normalizedRotation(settings.value[`camera.display.${deviceID}.rotation`])
  mirror.value = boolSetting(settings.value[`camera.display.${deviceID}.mirror`])
  flip.value = boolSetting(settings.value[`camera.display.${deviceID}.flip`])
  fitMode.value = settings.value[`camera.display.${deviceID}.fit_mode`] === 'cover' ? 'cover' : 'contain'
  cropX.value = boundedNumber(settings.value[`camera.display.${deviceID}.crop_x`], 0, 0, 99)
  cropY.value = boundedNumber(settings.value[`camera.display.${deviceID}.crop_y`], 0, 0, 99)
  cropWidth.value = boundedNumber(settings.value[`camera.display.${deviceID}.crop_width`], 100, 1, 100)
  cropHeight.value = boundedNumber(settings.value[`camera.display.${deviceID}.crop_height`], 100, 1, 100)
  pathPolicy.value = settings.value[`camera.path_policy.${deviceID}`] || 'auto'
}

function normalizedCrop() {
  const width = clamp(Number(cropWidth.value) || 100, 1, 100)
  const height = clamp(Number(cropHeight.value) || 100, 1, 100)
  return {
    x: clamp(Number(cropX.value) || 0, 0, 100 - width),
    y: clamp(Number(cropY.value) || 0, 0, 100 - height),
    width,
    height
  }
}

function normalizedRotation(raw?: string) {
  const value = Number(raw)
  return [0, 90, 180, 270].includes(value) ? value : 0
}

function boolSetting(raw?: string) {
  return raw === 'true' || raw === '1' || raw === 'yes' || raw === 'on'
}

function boundedNumber(raw: string | undefined, fallback: number, min: number, max: number) {
  const parsed = Number(raw)
  return clamp(Number.isFinite(parsed) ? parsed : fallback, min, max)
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}

onMounted(async () => {
  await loadDevice(String(route.params.id))
})

watch(() => route.params.id, (id) => {
  if (id) void loadDevice(String(id))
})

onBeforeUnmount(() => {
  stopStagePan?.()
})

async function loadDevice(id: string) {
  loading.value = true
  ready.value = false
  try {
    device.value = await api.device(id)
    credentials.value = await api.deviceCredentials(device.value.id)
    credentialIdentities.value = await api.credentialIdentities()
    bindings.value = await api.bindings()
    slots.value = await api.slots()
    settings.value = await api.settings()
    username.value = credentials.value.username || ''
    stream.value = credentials.value.stream || 'stream2'
    loadDisplaySettings()
    await loadRelayStatuses()
    credentialBaseline.value=JSON.stringify({username:username.value,password:password.value})
    editBaseline.value = editSnapshot.value
    ready.value = true
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Kamera konnte nicht geladen werden.'
  } finally {
    loading.value = false
  }
}

</script>

<style scoped>
.topline .meta.sig-row {
  display: flex;
  align-items: center;
  gap: 6px;
}
.transform-preview-stage.grabbable { cursor: grab; }
.transform-preview-stage.grabbable:active { cursor: grabbing; }
.stage-tools {
  position: absolute;
  top: 8px;
  left: 8px;
  z-index: 3;
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  width: max-content;
  max-width: calc(100% - 16px);
  padding: 4px;
  border-radius: var(--radius-sm);
  background: rgba(7, 7, 9, .62);
  backdrop-filter: blur(6px);
}
</style>
