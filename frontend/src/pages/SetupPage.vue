<template>
  <header class="topline">
    <div>
      <div class="eyebrow">Einrichtung · Werkbank</div>
      <h1 class="headline">Geräte <em>zu Plätzen</em> zuordnen.</h1>
    </div>
    <div class="meta">
      <div>Gefunden · <b>{{ devices.length }}</b></div>
      <div>Zugeordnet · <b>{{ boundCount }}/{{ slots.length }}</b></div>
    </div>
  </header>

  <div v-if="error" class="notice err"><span class="tag">FEHLER</span>{{ error }}</div>

  <div class="btn-row">
    <button class="btn primary" :disabled="busy === 'scan'" @click="runDiscovery">
      {{ busy === 'scan' ? 'Suche läuft…' : 'Netzwerk durchsuchen' }}
    </button>
    <button class="btn" :disabled="busy === 'render'" @click="render">go2rtc erzeugen</button>
    <button class="btn ghost" :disabled="busy === 'restart'" @click="restartGo2rtc">go2rtc neu starten</button>
    <div class="spacer" />
    <span v-if="busy === 'scan'" class="mono-mute" style="font-size: 11px;">RTSP · ONVIF · ARP</span>
  </div>
  <div v-if="busy === 'scan'" class="progress" />

  <div class="workbench">
    <!-- LEFT: device pool -->
    <section class="panel">
      <div class="panel-head">
        <h2>Gefundene Geräte</h2>
        <div class="right">{{ devices.length }} insgesamt</div>
      </div>
      <div v-if="!devices.length" class="empty">Noch keine Geräte. Starte die Suche oben.</div>
      <div v-else class="device-list">
        <button
          v-for="(device, ix) in devices"
          :key="device.id"
          class="device-card"
          :class="{ active: form.device_id === device.id }"
          @click="pickDevice(device.id)"
        >
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
                <option v-for="d in devices" :key="d.id" :value="d.id">
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

          <div class="btn-row" style="margin-top: 8px;">
            <button class="btn primary" :disabled="!form.device_id || saving" @click="save">Speichern</button>
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

  <div class="toast-host">
    <transition name="page"><div v-if="toast" class="toast" :key="toast">{{ toast }}</div></transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import type { Binding, Device, Slot } from '../types'

const route = useRoute()
const slots = ref<Slot[]>([])
const devices = ref<Device[]>([])
const bindings = ref<Binding[]>([])
const selectedSlot = ref('')
const rendered = ref('')
const renderInfo = ref('')
const toast = ref('')
const error = ref('')
const saving = ref(false)
const busy = ref<'' | 'scan' | 'render' | 'restart'>('')

const form = reactive({ device_id: '', label: '', username: '', stream_name: 'stream2' })
const slotLabel = computed(() => slots.value.find((s) => s.id === selectedSlot.value)?.label || 'Kamera')
const selectedDevice = computed(() => devices.value.find((d) => d.id === form.device_id))
const boundCount = computed(() => slots.value.filter((s) => bindings.value.some((b) => b.slot_id === s.id)).length)

function bindingFor(slotId: string) {
  return bindings.value.find((b) => b.slot_id === slotId)
}
function deviceTitle(d: Device) {
  return `${d.manufacturer || 'Unbekannt'} ${d.model || 'Kamera'}`.trim()
}
function sig(d: Device, key: string): boolean {
  const r = typeof d.raw_json === 'string' ? safeParse(d.raw_json) : (d.raw_json || {})
  return Boolean(r[key])
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
}
function pickDevice(id: string) {
  form.device_id = id
  if (!selectedSlot.value) {
    const firstEmpty = slots.value.find((s) => !bindingFor(s.id))
    if (firstEmpty) pickSlot(firstEmpty.id)
  }
}

async function load() {
  const status = await api.status()
  slots.value = status.slots
  devices.value = status.devices
  bindings.value = status.bindings
  if (!selectedSlot.value && slots.value.length) {
    const target = String(route.query.camera || '')
    if (target) {
      const slotOfDevice = bindings.value.find((b) => b.device_id === target)?.slot_id
      pickSlot(slotOfDevice || slots.value[0].id)
      form.device_id = target
    } else {
      pickSlot(slots.value[0].id)
    }
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

async function save() {
  if (!selectedSlot.value || !form.device_id) return
  saving.value = true
  try {
    await api.saveBinding({
      slot_id: selectedSlot.value,
      device_id: form.device_id,
      label: form.label,
      username: form.username,
      stream_name: form.stream_name,
      enabled: true
    })
    toast.value = 'Zuordnung gespeichert'
    setTimeout(() => (toast.value = ''), 2200)
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Speichern fehlgeschlagen.'
  } finally {
    saving.value = false
  }
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
