<template>
  <header class="topline">
    <div>
      <h1 class="headline">Kameras</h1>
    </div>
    <div class="meta">
      <div>Gefunden · <b>{{ assignableDevices.length }}</b></div>
      <div>Sichtbar · <b>{{ shownCount }}/{{ slots.length }}</b></div>
    </div>
  </header>

  <div v-if="error" class="notice err"><span class="tag">FEHLER</span>{{ error }}</div>

  <section class="service-strip">
    <div class="service">
      <div>
        <div class="name">Live-Ansicht</div>
        <div class="endpoint">Laufende Kamerabilder</div>
      </div>
      <span class="pill" :class="managerOnline ? 'live' : 'down'">{{ managerOnline ? 'aktiv' : 'offline' }}</span>
    </div>
    <div class="service">
      <div>
        <div class="name">go2rtc</div>
        <div class="endpoint">Streamdienst</div>
      </div>
      <span class="pill" :class="go2rtcOnline ? 'live' : 'down'">{{ go2rtcOnline ? 'aktiv' : 'offline' }}</span>
    </div>
    <div class="service">
      <div>
        <div class="name">Letzte Suche</div>
        <div class="endpoint">{{ lastScanRel }}</div>
      </div>
      <span class="pill" :class="shownCount ? 'live' : ''">{{ shownCount }} sichtbar</span>
    </div>
  </section>

  <div class="btn-row">
    <button class="btn primary" :disabled="busy === 'scan'" @click="runDiscovery">
      {{ busy === 'scan' ? 'Suche läuft…' : 'Kameras suchen' }}
    </button>
    <button class="btn" :disabled="!!busy || !shownCount" @click="refreshFrames">
      {{ busy === 'frames' ? 'Vorschau wird geladen…' : 'Vorschau aktualisieren' }}
    </button>
    <button class="btn icon" type="button" title="Kamera per RTSP hinzufügen" @click="showManualModal = true">+</button>
    <div class="spacer" />
    <span v-if="busy === 'scan'" class="mono-mute" style="font-size: 11px;">RTSP · ONVIF · ARP</span>
  </div>
  <div v-if="busy === 'scan'" class="progress" />

  <section v-if="blockingSlots.length" class="panel">
    <div class="panel-head">
      <h2>Braucht Aufmerksamkeit</h2>
      <div class="right">{{ blockingSlots.length }} Kamera{{ blockingSlots.length === 1 ? '' : 's' }}</div>
    </div>
    <div class="result-list">
      <div v-for="slot in blockingSlots" :key="slot.alias" class="result-row err">
        <span class="name">{{ slot.label }}</span>
        <span class="ip">{{ slot.device?.last_ip || 'keine IP' }}</span>
        <span class="stream">{{ slotStateLabel(slot.state) }}</span>
        <RouterLink class="action" :to="slot.binding?.device_id ? `/kamera/${slot.binding.device_id}` : '/einrichtung'">Öffnen</RouterLink>
        <span class="message">{{ slot.message }}</span>
      </div>
    </div>
  </section>

  <section class="panel">
    <div class="panel-head">
      <h2>Gefundene Kameras</h2>
      <div class="right">{{ assignableDevices.length }} Geräte · Klick öffnet Details</div>
    </div>
    <div v-if="!assignableDevices.length" class="empty">
      Noch keine Kameras. Starte die Suche oben oder füge eine RTSP-Kamera hinzu.
    </div>
    <div v-else class="device-list">
      <div
        v-for="(device, ix) in assignableDevices"
        :key="device.id"
        class="device-card"
        :class="{ active: isShown(device) }"
        role="button"
        tabindex="0"
        @click="goDetail(device.id)"
        @keydown.enter="goDetail(device.id)"
      >
        <img
          v-if="referenceVisible(device.id)"
          class="device-thumb"
          :src="referenceImageUrl(device.id)"
          alt=""
          @error="markReferenceMissing(device.id)"
        />
        <div v-else class="device-thumb empty" aria-label="Kein Bild gespeichert"></div>
        <div>
          <div class="title">
            <span class="ix">{{ String(ix + 1).padStart(2, '0') }}</span>
            <span>{{ deviceTitle(device) }}</span>
          </div>
          <div class="ip">{{ device.last_ip || 'IP unbekannt' }} · {{ device.mac_address || 'MAC unbekannt' }}</div>
        </div>
        <div class="device-card-actions" @click.stop>
          <div class="signals">
            <span class="sig" :class="{ on: sig(device, 'rtsp_port_open') }">RTSP</span>
            <span class="sig" :class="{ on: sig(device, 'onvif_port_open') }">ONVIF</span>
          </div>
          <button
            class="btn sm"
            :class="{ live: isShown(device) }"
            type="button"
            :disabled="busy === device.id"
            @click="toggleShow(device)"
          >
            {{ busy === device.id ? '…' : isShown(device) ? 'Sichtbar' : 'Anzeigen' }}
          </button>
        </div>
      </div>
    </div>
  </section>

  <div v-if="showManualModal" class="modal-backdrop" @click.self="closeManualModal">
    <form class="modal" @submit.prevent="addManual">
      <div class="modal-head">
        <div>
          <h2>Kamera per RTSP hinzufügen</h2>
        </div>
        <button class="btn icon sm ghost" type="button" title="Schließen" @click="closeManualModal">×</button>
      </div>
      <div class="split manual-add">
        <div class="field">
          <span class="lbl">IP-Adresse</span>
          <input v-model="manual.ip" placeholder="192.168.178.172" inputmode="numeric" autofocus />
        </div>
        <div class="field">
          <span class="lbl">Benutzername</span>
          <input v-model="manual.username" placeholder="Kamera-Benutzer" />
        </div>
        <div class="field">
          <span class="lbl">Passwort</span>
          <input v-model="manual.password" type="password" placeholder="Kamera-Passwort" />
        </div>
        <div class="field">
          <span class="lbl">Stream</span>
          <select v-model="manual.stream">
            <option value="stream2">stream2 · Live</option>
            <option value="stream1">stream1 · HD</option>
          </select>
        </div>
      </div>
      <div class="modal-foot">
        <span class="mono-mute">rtsp://IP:554/stream2 oder stream1</span>
        <div class="btn-row">
          <button class="btn ghost" type="button" @click="closeManualModal">Abbrechen</button>
          <button class="btn primary" type="submit" :disabled="busy === 'manual' || !manual.ip">
            {{ busy === 'manual' ? 'Wird geprüft…' : 'Hinzufügen' }}
          </button>
        </div>
      </div>
    </form>
  </div>

  <div class="toast-host">
    <transition name="page"><div v-if="toast" class="toast" :key="toast">{{ toast }}</div></transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'
import type { Binding, Device, Slot, StatusResponse, ViewerResponse, ViewerSlot, ViewerSlotState } from '../types'

const router = useRouter()
const slots = ref<Slot[]>([])
const devices = ref<Device[]>([])
const bindings = ref<Binding[]>([])
const systemStatus = ref<StatusResponse>()
const viewerData = ref<ViewerResponse>()
const error = ref('')
const toast = ref('')
const busy = ref('')
const frameRevision = ref(Date.now())
const missingReferences = ref<Record<string, boolean>>({})
const showManualModal = ref(false)
const manual = reactive({ ip: '', username: '', password: '', stream: 'stream2' })

const assignableDevices = computed(() => devices.value.filter((device) => isAssignableCamera(device)))
const shownCount = computed(() => bindings.value.filter((b) => b.device_id).length)
const go2rtcOnline = computed(() => systemStatus.value?.system.go2rtc.online ?? false)
const managerOnline = computed(() => systemStatus.value?.system.camera_appliance.online ?? true)
const blockingSlots = computed<ViewerSlot[]>(() =>
  (viewerData.value?.slots ?? []).filter((slot) => slot.state !== 'online' && slot.state !== 'connecting' && slot.state !== 'unassigned')
)
const lastScanRel = computed(() => {
  const started = systemStatus.value?.scan_runs?.[0]?.started_at
  if (!started) return 'noch nie'
  const diff = (Date.now() - new Date(started).getTime()) / 1000
  if (diff < 60) return 'gerade eben'
  if (diff < 3600) return `vor ${Math.floor(diff / 60)} Min.`
  if (diff < 86400) return `vor ${Math.floor(diff / 3600)} Std.`
  return new Date(started).toLocaleDateString('de-DE')
})

function bindingForDevice(deviceId: string) {
  return bindings.value.find((b) => b.device_id === deviceId)
}
function isShown(device: Device) {
  return !!bindingForDevice(device.id)
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
function deviceTitle(d: Device) {
  return `${d.manufacturer || 'Unbekannt'} ${d.model || 'Kamera'}`.trim()
}
function sig(d: Device, key: string): boolean {
  const r = typeof d.raw_json === 'string' ? safeParse(d.raw_json) : (d.raw_json || {})
  return Boolean(r[key])
}
function isAssignableCamera(d: Device): boolean {
  if (bindings.value.some((binding) => binding.device_id === d.id)) return true
  const raw = typeof d.raw_json === 'string' ? safeParse(d.raw_json) : (d.raw_json || {})
  return Boolean(raw.manual || raw.rtsp_port_open || raw.onvif_port_open)
}
function safeParse(v: string): Record<string, unknown> {
  try { return JSON.parse(v) as Record<string, unknown> } catch { return {} }
}
function slotStateLabel(state: ViewerSlotState) {
  const labels: Record<ViewerSlotState, string> = {
    unassigned: 'leer',
    connecting: 'verbindet',
    online: 'live',
    offline: 'offline',
    credentials_failed: 'Login',
    stream_unavailable: 'Stream'
  }
  return labels[state]
}

function goDetail(deviceId: string) {
  void router.push(`/kamera/${deviceId}`)
}

async function load() {
  const status = await api.status()
  systemStatus.value = status
  slots.value = status.slots
  devices.value = status.devices
  bindings.value = status.bindings
  viewerData.value = await api.viewer().catch(() => undefined)
}

async function runDiscovery() {
  busy.value = 'scan'
  error.value = ''
  try {
    const result = await api.discover()
    devices.value = result.devices
    toast.value = `${result.devices.length} Gerät(e) gefunden`
    setTimeout(() => (toast.value = ''), 2400)
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Suche fehlgeschlagen.'
  } finally {
    busy.value = ''
  }
}

async function toggleShow(device: Device) {
  if (busy.value) return
  busy.value = device.id
  error.value = ''
  try {
    const existing = bindingForDevice(device.id)
    if (existing) {
      await api.removeBinding(existing.slot_id)
    } else {
      const used = new Set(bindings.value.map((b) => b.slot_id))
      const free = slots.value.find((slot) => !used.has(slot.id))
      if (!free) {
        error.value = 'Maximal ' + slots.value.length + ' Kameras gleichzeitig sichtbar. Blende zuerst eine andere aus.'
        return
      }
      const credentials = await api.deviceCredentials(device.id).catch(() => undefined)
      await api.saveBinding({
        slot_id: free.id,
        device_id: device.id,
        label: deviceTitle(device),
        username: credentials?.username || '',
        stream_name: credentials?.stream || free.default_stream || 'stream2',
        enabled: true
      })
    }
    await api.renderGo2RTC()
    await api.restartGo2RTC().catch(() => undefined)
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Sichtbarkeit konnte nicht geändert werden.'
  } finally {
    busy.value = ''
  }
}

async function refreshFrames() {
  const targets = bindings.value.filter((b) => b.device_id && b.device?.last_ip)
  if (!targets.length) return
  busy.value = 'frames'
  error.value = ''
  let ok = 0
  let failed = 0
  try {
    for (const binding of targets) {
      try {
        const credentials = await api.deviceCredentials(binding.device_id).catch(() => undefined)
        await api.captureFrame(binding.device_id, {
          username: credentials?.username || binding.username || '',
          password: '',
          stream: credentials?.stream || binding.stream_name || 'stream2',
          save: true
        })
        ok += 1
        const next = { ...missingReferences.value }
        delete next[binding.device_id]
        missingReferences.value = next
      } catch {
        failed += 1
        missingReferences.value = { ...missingReferences.value, [binding.device_id]: true }
      }
    }
    frameRevision.value = Date.now()
    toast.value = failed ? `${ok} Vorschau(en) aktualisiert, ${failed} fehlgeschlagen` : `${ok} Vorschau(en) aktualisiert`
    setTimeout(() => (toast.value = ''), 2800)
  } finally {
    busy.value = ''
  }
}

async function addManual() {
  busy.value = 'manual'
  error.value = ''
  try {
    const result = await api.addManualDevice({
      ip: manual.ip,
      username: manual.username,
      password: manual.password,
      stream: manual.stream
    })
    if (manual.username && manual.password) {
      try {
        await api.captureFrame(result.device.id, { username: manual.username, password: manual.password, stream: manual.stream, save: true })
        const next = { ...missingReferences.value }
        delete next[result.device.id]
        missingReferences.value = next
        frameRevision.value = Date.now()
      } catch {
        // frame optional
      }
    }
    toast.value = result.message
    setTimeout(() => (toast.value = ''), 3200)
    manual.password = ''
    showManualModal.value = false
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Kamera konnte nicht hinzugefügt werden.'
  } finally {
    busy.value = ''
  }
}

function closeManualModal() {
  if (busy.value !== 'manual') showManualModal.value = false
}

onMounted(load)
</script>

<style scoped>
.device-card-actions {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 8px;
}
</style>
