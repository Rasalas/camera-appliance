<template>
  <header class="topline">
    <div><h1 class="headline">Kameras</h1><p v-if="loaded" class="mono-mute">{{ assignableDevices.length }} gefunden · {{ shownCount }}/{{ slots.length }} sichtbar</p></div>
    <div class="page-actions">
      <button class="btn ghost" :disabled="busy === 'scan' || !loaded" @click="runDiscovery"><AppIcon name="search" />{{ busy === 'scan' ? 'Suche läuft…' : 'Kameras suchen' }}</button>
      <button class="btn ghost" :disabled="!!busy || !shownCount" @click="refreshFrames">{{ busy === 'frames' ? 'Vorschau wird geladen…' : 'Vorschau aktualisieren' }}</button>
      <button class="btn primary desktop-primary" type="button" aria-label="Kamera hinzufügen" title="Kamera über ihre IP-Adresse hinzufügen" @click="openManualModal"><AppIcon name="plus" />Kamera hinzufügen</button>
    </div>
  </header>

  <div v-if="error" class="notice err" role="alert"><span class="tag">FEHLER</span>{{ error }}</div>

  <div v-if="loadError" class="notice err load-error" role="alert">
    <span class="tag">FEHLER</span>
    <span>Die Kameras konnten nicht {{ loaded ? 'aktualisiert' : 'geladen' }} werden. {{ loadError }}</span>
    <button class="btn" type="button" :disabled="loading" @click="load">{{ loading ? 'Wird geladen…' : 'Erneut versuchen' }}</button>
  </div>
  <div v-else-if="!loaded" class="empty" role="status">Kameras werden geladen…</div>

  <template v-if="loaded">
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
          <div class="name">Videodienst</div>
          <div class="endpoint">Überträgt die Kamerabilder</div>
        </div>
        <span class="pill" :class="go2rtcOnline ? 'live' : 'down'">{{ go2rtcOnline ? 'aktiv' : 'offline' }}</span>
      </div>
      <div class="service">
        <div>
          <div class="name">Letzte Suche</div>
          <div class="endpoint">{{ lastScanRel }}</div>
        </div>
        <span class="pill" :class="shownCount ? 'live' : ''">{{ countLabel(shownCount, 'Kamera', 'Kameras') }} sichtbar</span>
      </div>
    </section>

    <div v-if="busy === 'scan'" class="progress" />

    <section v-if="blockingSlots.length" class="panel">
      <div class="panel-head">
        <h2>Braucht Aufmerksamkeit</h2>
        <div class="right">{{ countLabel(blockingSlots.length, 'Kamera', 'Kameras') }}</div>
      </div>
      <div class="result-list">
        <div v-for="slot in blockingSlots" :key="slot.alias" class="result-row err">
          <span class="name">{{ slot.label }}</span>
          <span class="ip">{{ slot.device?.last_ip || 'keine IP-Adresse' }}</span>
          <span class="stream">{{ cameraStateLabel(slot.state) }}</span>
          <RouterLink class="action" :to="slot.binding?.device_id ? `/kamera/${slot.binding.device_id}` : '/einrichtung'">Öffnen</RouterLink>
          <span class="message">{{ slot.message }}</span>
        </div>
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>Gefundene Kameras</h2>
        <div class="right">{{ countLabel(assignableDevices.length, 'Kamera', 'Kameras') }} · nach Name sortiert</div>
      </div>
      <div v-if="!assignableDevices.length" class="empty">
        Noch keine Kameras. Starte die Suche oben oder füge eine Kamera über ihre IP-Adresse hinzu.
      </div>
      <div v-else class="device-list">
        <div
          v-for="(device, ix) in assignableDevices"
          :key="device.id"
          class="device-card"
          :class="{ active: isShown(device) }"
          @click="openDeviceRow($event, device.id)"
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
              <RouterLink :to="`/kamera/${device.id}`">{{ cameraName(device) }}</RouterLink>
            </div>
            <div class="ip">{{ deviceSubtitle(device) }}</div>
          </div>
          <div class="device-card-actions" @click.stop>
            <span class="pill" :class="{ live: isShown(device) }">{{ isShown(device) ? 'In der Live-Ansicht' : 'Ausgeblendet' }}</span>
            <div class="btn-row">
              <button class="btn sm ghost" type="button" :aria-label="`${cameraName(device)} umbenennen`" @click="renameTarget = device">Umbenennen</button>
              <button class="btn sm" type="button" :disabled="!!busy" :aria-label="`${cameraName(device)} ${isShown(device) ? 'ausblenden' : 'einblenden'}`" @click="askVisibility(device)">
                {{ isShown(device) ? 'Ausblenden' : 'Einblenden' }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </section>
  </template>

  <button class="mobile-fab" aria-label="Kamera hinzufügen" @click="openManualModal"><AppIcon name="plus" /></button>
  <AdminDialog ref="manualDialog" :open="showManualModal" title="Kamera manuell hinzufügen" :dirty="manualDirty" :busy="busy === 'manual'" @close="showManualModal=false">
    <form @submit.prevent="addManual">
      <div class="split manual-add">
        <label class="field">
          <span class="lbl">IP-Adresse</span>
          <input aria-label="IP-Adresse" v-model="manual.ip" required placeholder="192.168.178.172" autofocus />
        </label>
        <label class="field">
          <span class="lbl">Benutzername</span>
          <input aria-label="Benutzername" v-model="manual.username" placeholder="Kamera-Benutzer" />
        </label>
        <label class="field">
          <span class="lbl">Passwort</span>
          <input aria-label="Passwort" v-model="manual.password" type="password" placeholder="Kamera-Passwort" />
        </label>
        <label class="field">
          <span class="lbl">Bildqualität</span>
          <select aria-label="Bildqualität" v-model="manual.stream">
            <option value="stream2">{{ streamLabel('stream2') }}</option>
            <option value="stream1">{{ streamLabel('stream1') }}</option>
          </select>
        </label>
      </div>
      <p v-if="manualError" class="notice err" role="alert">{{ manualError }}</p>
      <div class="modal-foot">
        <span class="mono-mute">Benutzername und Passwort sind die Zugangsdaten, die in der Kamera selbst eingerichtet sind.</span>
        <div class="btn-row">
          <button class="btn ghost" type="button" @click="closeManualModal">Abbrechen</button>
          <button class="btn primary" type="submit" :disabled="busy === 'manual' || !manual.ip"><AppIcon name="plus" />
            {{ busy === 'manual' ? 'Wird geprüft…' : 'Hinzufügen' }}
          </button>
        </div>
      </div>
    </form>
  </AdminDialog>

  <CameraNameDialog :open="!!renameTarget" :device="renameTarget" @close="renameTarget = undefined" @saved="onRenamed" />
  <CameraVisibilityDialog :open="!!visibilityTarget" :name="visibilityTarget ? cameraName(visibilityTarget) : ''" :shown="!!visibilityTarget && isShown(visibilityTarget)" :busy="busy === 'visibility'" :error="visibilityError" @close="closeVisibility" @confirm="confirmVisibility" />

  <div class="toast-host">
    <transition name="page"><div v-if="toast" class="toast" :key="toast">{{ toast }}</div></transition>
  </div>
</template>

<script setup lang="ts">
import AppIcon from '../components/AppIcon.vue'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import AdminDialog from '../components/AdminDialog.vue'
import CameraNameDialog from '../components/CameraNameDialog.vue'
import CameraVisibilityDialog from '../components/CameraVisibilityDialog.vue'
import { useDraftGuard } from '../composables/discardChanges'
import { isCameraResource } from '../composables/cameraResources'
import { cameraHardwareLabel, cameraName, cameraStateLabel, changeCameraVisibility, countLabel, streamLabel } from '../composables/cameraLabels'
import { rowDestination } from '../composables/resourceRow'
import { api } from '../api/client'
import type { Binding, Device, Slot, StatusResponse, ViewerResponse, ViewerSlot } from '../types'

const router = useRouter()
const slots = ref<Slot[]>([])
const devices = ref<Device[]>([])
const bindings = ref<Binding[]>([])
const systemStatus = ref<StatusResponse>()
const viewerData = ref<ViewerResponse>()
const error = ref('')
const loadError = ref('')
const loading = ref(false)
const loaded = computed(() => !!systemStatus.value)
const manualError = ref('')
const renameTarget = ref<Device>()
const visibilityTarget = ref<Device>()
const visibilityError = ref('')
const toast = ref('')
const busy = ref('')
const frameRevision = ref(Date.now())
const missingReferences = ref<Record<string, boolean>>({})
const showManualModal = ref(false)
const manualDialog=ref<InstanceType<typeof AdminDialog>>(), manualBaseline=ref('')
const manualDirty=computed(()=>showManualModal.value && JSON.stringify(manual)!==manualBaseline.value)
watch(showManualModal,open=>{if(open)manualBaseline.value=JSON.stringify(manual)})
useDraftGuard(()=>manualDirty.value,()=>{showManualModal.value=false})
function openDeviceRow(event:MouseEvent,id:string) { const href=rowDestination(event,`/kamera/${id}`);if(href)void router.push(href) }
const manual = reactive({ ip: '', username: '', password: '', stream: 'stream2' })

const assignableDevices = computed(() => devices.value.filter((device) => isCameraResource(device, bindings.value)).sort((a,b)=>cameraName(a).localeCompare(cameraName(b),'de')))
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
function deviceSubtitle(d: Device) {
  const ip = d.last_ip ? `IP-Adresse ${d.last_ip}` : 'IP-Adresse unbekannt'
  return d.name?.trim() && cameraHardwareLabel(d) ? `${cameraHardwareLabel(d)} · ${ip}` : ip
}

async function load() {
  loading.value = true
  try {
    const status = await api.status()
    systemStatus.value = status
    slots.value = status.slots
    devices.value = status.devices
    bindings.value = status.bindings
    loadError.value = ''
    viewerData.value = await api.viewer().catch(() => undefined)
  } catch (err) {
    loadError.value = err instanceof Error ? err.message : ''
  } finally {
    loading.value = false
  }
}

function onRenamed(updated: Device) {
  devices.value = devices.value.map((device) => device.id === updated.id ? { ...device, name: updated.name } : device)
  toast.value = 'Name gespeichert'
  setTimeout(() => (toast.value = ''), 2400)
}

async function runDiscovery() {
  busy.value = 'scan'
  error.value = ''
  try {
    const result = await api.discover()
    devices.value = result.devices
    toast.value = `${countLabel(result.devices.length, 'Gerät', 'Geräte')} gefunden`
    setTimeout(() => (toast.value = ''), 2400)
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Suche fehlgeschlagen.'
  } finally {
    busy.value = ''
  }
}

function askVisibility(device: Device) {
  visibilityError.value = ''
  visibilityTarget.value = device
}
function closeVisibility() {
  if (busy.value !== 'visibility') visibilityTarget.value = undefined
}
async function confirmVisibility() {
  const device = visibilityTarget.value
  if (!device || busy.value) return
  busy.value = 'visibility'
  visibilityError.value = ''
  error.value = ''
  try {
    const credentials = isShown(device) ? undefined : await api.deviceCredentials(device.id).catch(() => undefined)
    const result = await changeCameraVisibility(api, {
      deviceId: device.id,
      label: cameraName(device),
      bindings: bindings.value,
      slots: slots.value,
      username: credentials?.username,
      stream: credentials?.stream
    })
    visibilityTarget.value = undefined
    error.value = result.restartError
    await load()
  } catch (err) {
    visibilityError.value = err instanceof Error ? err.message : 'Die Anzeige konnte nicht geändert werden.'
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
    const updated = `${countLabel(ok, 'Vorschau', 'Vorschauen')} aktualisiert`
    toast.value = failed ? `${updated}, ${failed} fehlgeschlagen` : updated
    setTimeout(() => (toast.value = ''), 2800)
  } finally {
    busy.value = ''
  }
}

function openManualModal() {
  manualError.value = ''
  showManualModal.value = true
}

async function addManual() {
  busy.value = 'manual'
  manualError.value = ''
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
    manualError.value = err instanceof Error ? err.message : 'Kamera konnte nicht hinzugefügt werden.'
  } finally {
    busy.value = ''
  }
}

function closeManualModal() {
  void manualDialog.value?.requestClose()
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
.load-error { align-items: center; }
.load-error > span:not(.tag) { flex: 1; }
</style>
