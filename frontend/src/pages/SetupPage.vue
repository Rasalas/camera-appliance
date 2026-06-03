<template>
  <header class="topline">
    <div>
      <div class="eyebrow">Einrichtung · Werkbank</div>
      <h1 class="headline">Geräte <em>zu Plätzen</em> zuordnen.</h1>
    </div>
    <div class="meta">
      <div>Gefunden · <b>{{ assignableDevices.length }}</b></div>
      <div>Zugeordnet · <b>{{ boundCount }}/{{ slots.length }}</b></div>
    </div>
  </header>

  <div v-if="error" class="notice err"><span class="tag">FEHLER</span>{{ error }}</div>

  <div class="btn-row">
    <button class="btn primary" :disabled="busy === 'scan'" @click="runDiscovery">
      {{ busy === 'scan' ? 'Suche läuft…' : 'Netzwerk durchsuchen' }}
    </button>
    <button class="btn" :disabled="!!busy || boundCount === 0" @click="refreshFrames">
      {{ busy === 'frames' ? 'Bilder werden gezogen…' : 'Bilder aktualisieren' }}
    </button>
    <button class="btn" :disabled="busy === 'render'" @click="render">go2rtc erzeugen</button>
    <button class="btn ghost" :disabled="busy === 'restart'" @click="restartGo2rtc">go2rtc neu starten</button>
    <div class="spacer" />
    <span v-if="busy === 'scan'" class="mono-mute" style="font-size: 11px;">RTSP · ONVIF · ARP</span>
  </div>
  <div v-if="busy === 'scan'" class="progress" />

  <section v-if="frameResults.length" class="panel">
    <div class="panel-head">
      <h2>Bildprüfung</h2>
      <div class="right">{{ frameResults.filter((r) => r.success).length }}/{{ frameResults.length }} erfolgreich</div>
    </div>
    <div class="result-list">
      <div v-for="result in frameResults" :key="result.slot_id" class="result-row" :class="{ ok: result.success, err: !result.success }">
        <span class="slot">{{ result.slot_id }}</span>
        <span class="name">{{ result.label }}</span>
        <span class="ip">{{ result.ip || 'ohne IP' }}</span>
        <span class="stream">{{ result.stream || '—' }}</span>
        <RouterLink v-if="!result.success" class="action" :to="`/kamera/${result.device_id}`">Diagnose</RouterLink>
        <span v-else class="action">OK</span>
        <span class="message">{{ result.message }}</span>
      </div>
    </div>
  </section>

  <div class="workbench">
    <!-- LEFT: device pool -->
    <section class="panel">
      <div class="panel-head">
        <h2>Gefundene Geräte</h2>
        <div class="device-head-actions">
          <div class="right">{{ assignableDevices.length }} Kameras</div>
          <button class="btn icon sm" type="button" title="Kamera per RTSP hinzufügen" @click="showManualModal = true">+</button>
        </div>
      </div>
      <div v-if="!assignableDevices.length" class="empty">Noch keine Kameras. Starte die Suche oben oder füge eine RTSP-Kamera hinzu.</div>
      <div v-else class="device-list">
        <button
          v-for="(device, ix) in assignableDevices"
          :key="device.id"
          class="device-card"
          :class="{ active: form.device_id === device.id }"
          @click="pickDevice(device.id)"
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
          <div class="signals">
            <span class="sig" :class="{ on: sig(device, 'rtsp_port_open') }">RTSP</span>
            <span class="sig" :class="{ on: sig(device, 'onvif_port_open') }">ONVIF</span>
          </div>
        </button>
      </div>
    </section>

    <!-- RIGHT: layout + form -->
    <section style="display: grid; gap: 24px;">
      <div class="panel">
        <div class="panel-head">
          <h2>Anzeige-Layout</h2>
          <div class="right">{{ selectedSlot ? `Aktiv: ${selectedSlot}` : 'Platz wählen' }}</div>
        </div>
        <div class="layout-canvas">
          <button
            v-for="slot in slots"
            :key="slot.id"
            class="bay"
            :class="{
              large: slot.role === 'large',
              bound: !!bindingFor(slot.id),
              target: selectedSlot === slot.id
            }"
            @click="pickSlot(slot.id)"
          >
            <img
              v-if="referenceVisible(bindingFor(slot.id)?.device_id)"
              class="bay-reference"
              :src="referenceImageUrl(bindingFor(slot.id)?.device_id)"
              alt=""
              @error="markReferenceMissing(bindingFor(slot.id)?.device_id)"
            />
            <div class="bay-id">{{ slot.id }}</div>
            <div>
              <div class="bay-name">{{ bindingFor(slot.id)?.label || slot.label }}</div>
              <div v-if="bindingFor(slot.id)?.device?.last_ip" class="bay-ip">{{ bindingFor(slot.id)?.device?.last_ip }}</div>
              <div v-else class="bay-empty">— leer —</div>
            </div>
          </button>
        </div>
      </div>

      <div class="panel">
        <div class="panel-head">
          <h2>Zuordnung · {{ selectedSlot || '—' }}</h2>
          <div v-if="selectedDevice" class="right">{{ selectedDevice.last_ip || 'ohne IP' }}</div>
        </div>

        <div v-if="!selectedSlot" class="empty">Wähle einen Platz im Layout oben.</div>
        <template v-else>
          <div class="split">
            <div class="field">
              <span class="lbl">Gerät</span>
              <select v-model="form.device_id">
                <option value="">— kein Gerät —</option>
                <option v-for="d in assignableDevices" :key="d.id" :value="d.id">
                  {{ deviceTitle(d) }} · {{ d.last_ip || 'ohne IP' }}
                </option>
              </select>
            </div>
            <div class="field">
              <span class="lbl">Anzeigename</span>
              <input v-model="form.label" :placeholder="slotLabel" />
            </div>
            <div class="field">
              <span class="lbl">Benutzername (Kamera)</span>
              <input v-model="form.username" placeholder="tapo_hof" />
            </div>
            <div class="field">
              <span class="lbl">Stream</span>
              <select v-model="form.stream_name">
                <option value="stream2">stream2 · empfohlen</option>
                <option value="stream1">stream1</option>
              </select>
            </div>
          </div>

          <div v-if="form.device_id" class="assignment-display">
            <div class="assignment-display-head">
              <div>
                <div class="lbl">Anzeige</div>
                <div class="mono-mute">{{ displaySummary }}</div>
              </div>
              <RouterLink class="btn sm ghost" :to="`/kamera/${form.device_id}`">Crop/Diagnose →</RouterLink>
            </div>

            <div class="assignment-display-grid">
              <div class="field">
                <span class="lbl">Rotation</span>
                <div class="btn-row">
                  <button
                    v-for="value in [0, 90, 180, 270]"
                    :key="value"
                    class="btn sm"
                    :class="{ live: rotation === value }"
                    type="button"
                    @click="rotation = value"
                  >
                    {{ value }}°
                  </button>
                </div>
              </div>

              <label class="toggle-row compact">
                <input v-model="mirror" type="checkbox" />
                <div>
                  <div class="lbl-main">Horizontal spiegeln</div>
                  <div class="lbl-sub">Links/rechts tauschen.</div>
                </div>
              </label>

              <label class="toggle-row compact">
                <input v-model="flip" type="checkbox" />
                <div>
                  <div class="lbl-main">Vertikal spiegeln</div>
                  <div class="lbl-sub">Kopfstehende Montage.</div>
                </div>
              </label>

              <div class="field">
                <span class="lbl">Fit</span>
                <select v-model="fitMode">
                  <option value="cover">Cover</option>
                  <option value="contain">Contain</option>
                </select>
              </div>
            </div>
          </div>

          <div class="btn-row" style="margin-top: 8px;">
            <button class="btn primary" :disabled="!form.device_id || saving" @click="save">Speichern</button>
            <button class="btn" :disabled="!form.device_id || saving" @click="saveDisplay">Anzeige speichern</button>
            <button class="btn danger" :disabled="!bindingFor(selectedSlot)" @click="remove">Entfernen</button>
            <RouterLink v-if="form.device_id" class="btn ghost" :to="`/kamera/${form.device_id}`">Diagnose →</RouterLink>
          </div>
        </template>
      </div>
    </section>
  </div>

  <section v-if="rendered" class="panel">
    <div class="panel-head">
      <h2>Erzeugte go2rtc-Konfiguration</h2>
      <div class="right">{{ renderInfo }}</div>
    </div>
    <pre class="code">{{ rendered }}</pre>
  </section>

  <div v-if="showManualModal" class="modal-backdrop" @click.self="closeManualModal">
    <form class="modal" @submit.prevent="addManual">
      <div class="modal-head">
        <div>
          <div class="eyebrow">Gefundene Geräte</div>
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
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import type { Binding, Device, Slot } from '../types'

interface FrameRefreshResult {
  slot_id: string
  device_id: string
  label: string
  ip: string
  stream: string
  success: boolean
  message: string
}

const route = useRoute()
const slots = ref<Slot[]>([])
const devices = ref<Device[]>([])
const bindings = ref<Binding[]>([])
const settings = ref<Record<string, string>>({})
const selectedSlot = ref('')
const rendered = ref('')
const renderInfo = ref('')
const toast = ref('')
const error = ref('')
const saving = ref(false)
const busy = ref<'' | 'scan' | 'render' | 'restart' | 'manual' | 'frames'>('')
const frameRevision = ref(Date.now())
const missingReferences = ref<Record<string, boolean>>({})
const showManualModal = ref(false)
const frameResults = ref<FrameRefreshResult[]>([])

const form = reactive({ device_id: '', label: '', username: '', stream_name: 'stream2' })
const manual = reactive({ ip: '', username: '', password: '', stream: 'stream2' })
const rotation = ref(0)
const mirror = ref(false)
const flip = ref(false)
const fitMode = ref<'cover' | 'contain'>('cover')
const slotLabel = computed(() => slots.value.find((s) => s.id === selectedSlot.value)?.label || 'Kamera')
const selectedDevice = computed(() => devices.value.find((d) => d.id === form.device_id))
const boundCount = computed(() => slots.value.filter((s) => bindings.value.some((b) => b.slot_id === s.id)).length)
const assignableDevices = computed(() => devices.value.filter((device) => isAssignableCamera(device)))
const displaySummary = computed(() => `${rotation.value}° · ${fitMode.value}${mirror.value ? ' · gespiegelt' : ''}${flip.value ? ' · vertikal' : ''}`)

function bindingFor(slotId: string) {
  return bindings.value.find((b) => b.slot_id === slotId)
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

function pickSlot(slotId: string) {
  selectedSlot.value = slotId
  const b = bindingFor(slotId)
  form.device_id = b?.device_id || form.device_id || ''
  form.label = b?.label || slots.value.find((s) => s.id === slotId)?.label || ''
  form.username = b?.username || ''
  form.stream_name = b?.stream_name || 'stream2'
  loadDisplaySettingsForDevice(form.device_id)
}
function pickDevice(id: string) {
  form.device_id = id
  loadDisplaySettingsForDevice(id)
  void loadDeviceCredentials(id)
  if (!selectedSlot.value) {
    const firstEmpty = slots.value.find((s) => !bindingFor(s.id))
    if (firstEmpty) pickSlot(firstEmpty.id)
  }
}

async function loadDeviceCredentials(id: string) {
  try {
    const credentials = await api.deviceCredentials(id)
    if (!form.username) form.username = credentials.username || ''
    if (credentials.stream) form.stream_name = credentials.stream
  } catch {
    // credentials are optional; keep the assignment flow usable.
  }
}

async function load() {
  const status = await api.status()
  slots.value = status.slots
  devices.value = status.devices
  bindings.value = status.bindings
  settings.value = await api.settings()
  if (!selectedSlot.value && slots.value.length) {
    const target = String(route.query.camera || '')
    if (target) {
      const slotOfDevice = bindings.value.find((b) => b.device_id === target)?.slot_id
      pickSlot(slotOfDevice || slots.value[0].id)
      form.device_id = target
    } else {
      pickSlot(slots.value[0].id)
    }
  } else if (form.device_id) {
    loadDisplaySettingsForDevice(form.device_id)
  }
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
    await load()
    form.device_id = result.device.id
    form.username = manual.username
    form.stream_name = manual.stream
    if (!form.label) form.label = selectedSlot.value ? slotLabel.value : 'Kamera'
    await loadDeviceCredentials(result.device.id)
    let frameNote = ''
    if (manual.username && manual.password) {
      try {
        await api.captureFrame(result.device.id, {
          username: manual.username,
          password: manual.password,
          stream: manual.stream,
          save: true
        })
        const next = { ...missingReferences.value }
        delete next[result.device.id]
        missingReferences.value = next
        frameRevision.value = Date.now()
        frameNote = ' Referenzbild gespeichert.'
      } catch (err) {
        frameNote = ` Bild noch nicht verfügbar: ${err instanceof Error ? err.message : 'Frame fehlgeschlagen'}`
      }
    }
    toast.value = `${result.message}${frameNote}`
    setTimeout(() => (toast.value = ''), 4200)
    manual.password = ''
    showManualModal.value = false
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Kamera konnte nicht hinzugefügt werden.'
  } finally {
    busy.value = ''
  }
}

async function refreshFrames() {
  const targets = bindings.value.filter((b) => b.device_id && b.device?.last_ip)
  if (!targets.length) return
  busy.value = 'frames'
  error.value = ''
  frameResults.value = []
  let ok = 0
  let failed = 0
  try {
    for (const binding of targets) {
      const result = await refreshFrame(binding)
      frameResults.value = [...frameResults.value, result]
      if (result.success) {
        ok += 1
        const next = { ...missingReferences.value }
        delete next[binding.device_id]
        missingReferences.value = next
      } else {
        failed += 1
        missingReferences.value = { ...missingReferences.value, [binding.device_id]: true }
      }
    }
    frameRevision.value = Date.now()
    toast.value = failed ? `${ok} Bild(er) aktualisiert, ${failed} fehlgeschlagen` : `${ok} Bild(er) aktualisiert`
    setTimeout(() => (toast.value = ''), 2800)
    if (!ok && failed) error.value = 'Keine Bilder konnten gezogen werden. Prüfe Zugangsdaten und RTSP-Freigabe je Kamera.'
  } finally {
    busy.value = ''
  }
}

async function refreshFrame(binding: Binding): Promise<FrameRefreshResult> {
  const credentials = await api.deviceCredentials(binding.device_id).catch(() => undefined)
  const username = credentials?.username || binding.username || ''
  const preferred = credentials?.stream || binding.stream_name || 'stream2'
  const streams = uniqueStreams([preferred, preferred === 'stream1' ? 'stream2' : 'stream1'])
  const messages: string[] = []

  for (const streamName of streams) {
    try {
      const frame = await api.captureFrame(binding.device_id, {
        username,
        password: '',
        stream: streamName,
        save: true
      })
      if (streamName !== preferred) {
        await persistWorkingStream(binding, username, streamName)
      }
      const source = frame.credential_source ? ` über ${frame.credential_source}` : ''
      return frameResult(binding, streamName, true, streamName === preferred ? `Bild gespeichert${source}` : `Bild gespeichert${source}, ${streamName} übernommen`)
    } catch (err) {
      messages.push(`${streamName}: ${err instanceof Error ? err.message : 'fehlgeschlagen'}`)
    }
  }

  return frameResult(binding, preferred, false, messages.join(' · ') || 'Kein Frame erhalten')
}

function frameResult(binding: Binding, streamName: string, success: boolean, message: string): FrameRefreshResult {
  const duplicates = duplicateSlotsForIP(binding)
  const conflict = duplicates.length ? `Doppelte IP mit ${duplicates.join(', ')}. ` : ''
  return {
    slot_id: binding.slot_id,
    device_id: binding.device_id,
    label: binding.label || binding.slot?.label || binding.device?.hostname || 'Kamera',
    ip: binding.device?.last_ip || '',
    stream: streamName,
    success,
    message: conflict + message
  }
}

function duplicateSlotsForIP(binding: Binding) {
  const ip = binding.device?.last_ip
  if (!ip) return []
  return bindings.value
    .filter((other) => other.slot_id !== binding.slot_id && other.device?.last_ip === ip)
    .map((other) => other.slot_id)
}

async function persistWorkingStream(binding: Binding, username: string, streamName: string) {
  if (username) {
    await api.saveDeviceCredentials(binding.device_id, { username, stream: streamName }).catch(() => undefined)
  }
  await api.saveBinding({
    slot_id: binding.slot_id,
    device_id: binding.device_id,
    label: binding.label,
    username: binding.username || username,
    stream_name: streamName,
    enabled: binding.enabled
  }).catch(() => undefined)
}

function uniqueStreams(values: string[]) {
  return Array.from(new Set(values.filter(Boolean)))
}

function loadDisplaySettingsForDevice(deviceID: string) {
  if (!deviceID) {
    resetDisplayControls()
    return
  }
  rotation.value = normalizedRotation(settings.value[`camera.display.${deviceID}.rotation`])
  mirror.value = boolSetting(settings.value[`camera.display.${deviceID}.mirror`])
  flip.value = boolSetting(settings.value[`camera.display.${deviceID}.flip`])
  fitMode.value = settings.value[`camera.display.${deviceID}.fit_mode`] === 'contain' ? 'contain' : 'cover'
}

function resetDisplayControls() {
  rotation.value = 0
  mirror.value = false
  flip.value = false
  fitMode.value = 'cover'
}

function normalizedRotation(raw?: string) {
  const value = Number(raw)
  return [0, 90, 180, 270].includes(value) ? value : 0
}

function boolSetting(raw?: string) {
  return raw === 'true' || raw === '1' || raw === 'yes' || raw === 'on'
}

function closeManualModal() {
  if (busy.value !== 'manual') showManualModal.value = false
}

async function save() {
  if (!selectedSlot.value || !form.device_id) return
  saving.value = true
  try {
    if (form.username || form.stream_name) {
      await api.saveDeviceCredentials(form.device_id, { username: form.username, stream: form.stream_name })
    }
    await api.saveBinding({
      slot_id: selectedSlot.value,
      device_id: form.device_id,
      label: form.label,
      username: form.username,
      stream_name: form.stream_name,
      enabled: true
    })
    await saveDisplaySettings(false)
    toast.value = 'Zuordnung gespeichert'
    setTimeout(() => (toast.value = ''), 2200)
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Speichern fehlgeschlagen.'
  } finally {
    saving.value = false
  }
}

async function saveDisplay() {
  if (!form.device_id) return
  saving.value = true
  error.value = ''
  try {
    await saveDisplaySettings(true)
    toast.value = 'Anzeige gespeichert'
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Anzeige konnte nicht gespeichert werden.'
  } finally {
    saving.value = false
  }
}

async function saveDisplaySettings(updateLocal: boolean) {
  if (!form.device_id) return
  const deviceID = form.device_id
  const values: Record<string, string> = {
    [`camera.display.${deviceID}.rotation`]: String(normalizedRotation(String(rotation.value))),
    [`camera.display.${deviceID}.mirror`]: String(mirror.value),
    [`camera.display.${deviceID}.flip`]: String(flip.value),
    [`camera.display.${deviceID}.fit_mode`]: fitMode.value
  }
  await api.saveSettings(values)
  if (updateLocal) Object.assign(settings.value, values)
}

async function remove() {
  if (!selectedSlot.value) return
  try {
    await api.removeBinding(selectedSlot.value)
    toast.value = 'Platz geleert'
    setTimeout(() => (toast.value = ''), 2200)
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Entfernen fehlgeschlagen.'
  }
}

async function render() {
  busy.value = 'render'
  try {
    const result = await api.renderGo2RTC()
    rendered.value = result.redacted_yaml
    renderInfo.value = `${result.rendered_streams} Stream(s)`
    toast.value = `${result.rendered_streams} Stream(s) erzeugt`
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'go2rtc-Erzeugung fehlgeschlagen.'
  } finally {
    busy.value = ''
  }
}

async function restartGo2rtc() {
  busy.value = 'restart'
  try {
    await api.restartGo2RTC()
    toast.value = 'go2rtc neu gestartet'
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Neustart fehlgeschlagen.'
  } finally {
    busy.value = ''
  }
}

onMounted(load)
</script>
