<template>
  <section class="panel card">
    <div class="panel-head">
      <h2>Relay</h2>
      <div class="device-head-actions">
        <div class="right">{{ relayIds.length ? `${relayIds.length} eingerichtet` : 'nicht eingerichtet' }}</div>
        <button class="btn sm primary" type="button" @click="openRelayModal">Relay hinzufügen</button>
      </div>
    </div>

    <div class="mono-mute">
      Ein Relay leitet Kamera-Streams per SSH über einen anderen Host. Einmal eingerichtet, steht es jeder Kamera
      automatisch als Ersatzpfad zur Verfügung — die Ports werden pro Kameraplatz automatisch vergeben, und der
      Watchdog wechselt bei Verbindungsproblemen von selbst auf das Relay und wieder zurück.
    </div>

    <div v-if="!relayIds.length" class="empty">Kein Relay eingerichtet. Direkt erreichbare Kameras funktionieren auch ohne.</div>
    <div v-else class="relay-config-list">
      <div v-for="relayId in relayIds" :key="relayId" class="relay-config">
        <div class="relay-config-head">
          <div>
            <div class="slot">Relay</div>
            <div class="name">{{ relayName(relayId) }}</div>
            <div class="mono-mute">{{ relayId }} · SSH {{ settings[relaySettingKey(relayId, 'ssh_target')] || relayId }}</div>
          </div>
          <div class="btn-row">
            <button v-if="relayStatusFor(relayId)?.process_state !== 'running'" class="btn sm" type="button" :disabled="relayActionBusy === `start:${relayId}`" @click="onRelayAction(relayId, 'start')">{{ relayActionBusy === `start:${relayId}` ? 'Startet…' : 'Start' }}</button>
            <button v-else class="btn sm ghost" type="button" :disabled="relayActionBusy === `restart:${relayId}`" @click="onRelayAction(relayId, 'restart')">{{ relayActionBusy === `restart:${relayId}` ? 'Startet…' : 'Restart' }}</button>
            <button class="btn sm ghost" type="button" :disabled="relayActionBusy === `stop:${relayId}`" @click="onRelayAction(relayId, 'stop')">{{ relayActionBusy === `stop:${relayId}` ? 'Stoppt…' : 'Stop' }}</button>
            <button class="btn sm danger" type="button" @click="onRemoveRelay(relayId)">Entfernen</button>
          </div>
        </div>

        <div class="relay-runtime" :class="relayStateClass(relayId)">
          <span class="state-dot"></span>
          <div>
            <div class="lbl-main">{{ relayStateLabel(relayId) }}</div>
            <div class="lbl-sub">{{ relayStatusFor(relayId)?.message || 'Noch kein Status.' }}</div>
            <div v-if="relayStatusFor(relayId)?.last_error" class="mono-mute">Fehler · {{ relayStatusFor(relayId)?.last_error }}</div>
          </div>
          <div class="mono-mute">{{ relayStatusFor(relayId)?.pid ? `PID ${relayStatusFor(relayId)?.pid}` : '' }}</div>
        </div>

        <label class="toggle-row compact">
          <input type="checkbox" :checked="relayAutoStart(relayId)" @change="onAutoStartChange(relayId, $event)" />
          <div><div class="lbl-main">Automatisch aktiv halten</div><div class="lbl-sub">Watchdog startet das Relay bei Ausfall erneut (empfohlen).</div></div>
        </label>

        <details class="advanced">
          <summary>Feinjustage</summary>
          <div class="relay-config-grid" style="margin-top: 12px;">
            <div class="field"><span class="lbl">Name</span><input v-model="settings[relaySettingKey(relayId, 'name')]" class="compact-input" :placeholder="relayId" /></div>
            <div class="field"><span class="lbl">SSH-Ziel</span><input v-model="settings[relaySettingKey(relayId, 'ssh_target')]" class="compact-input" placeholder="nas oder user@nas" /></div>
            <div class="field"><span class="lbl">Host aus go2rtc-Docker</span><input v-model="settings[relaySettingKey(relayId, 'host')]" class="compact-input" placeholder="host.docker.internal" /></div>
            <div class="field"><span class="lbl">Bind-Adresse</span><input v-model="settings[relaySettingKey(relayId, 'bind_host')]" class="compact-input" placeholder="127.0.0.1" /></div>
            <div class="field"><span class="lbl">Port-Basis</span><input v-model="settings[relaySettingKey(relayId, 'port_base')]" class="compact-input" :placeholder="String(relayPortBaseFallback(relayId))" /></div>
          </div>
          <div class="btn-row" style="margin-top: 12px;">
            <button class="btn sm primary" type="button" @click="saveRelayConfig(relayId)">Speichern & neu starten</button>
            <span class="mono-mute" style="font-size: 11px;">Log · {{ relayStatusFor(relayId)?.log_path || '—' }}</span>
          </div>
        </details>
      </div>
    </div>
  </section>

  <section class="panel card">
    <div class="panel-head">
      <h2>Kamera-Pfade</h2>
      <div class="right">Änderungen werden sofort gespeichert</div>
    </div>

    <div class="mono-mute">
      Standard ist <b>Automatisch</b>: Die Verbindung läuft direkt zur Kamera; fällt sie aus, wechselt der Watchdog
      selbständig auf das Relay und nach Erholung wieder zurück. Erzwinge einen Weg nur, wenn eine Kamera dauerhaft
      nur über einen davon erreichbar ist.
    </div>

    <div v-if="!cameraBindings.length" class="empty">Noch keine Kameras aktiviert.</div>
    <div v-else class="relay-camera-list">
      <div v-for="binding in cameraBindings" :key="binding.device_id" class="relay-camera">
        <div class="relay-camera-main">
          <div>
            <div class="name">{{ binding.label || binding.slot?.label || binding.slot_id }}</div>
            <div class="mono-mute">{{ binding.device?.last_ip || 'keine IP' }} · {{ binding.slot_id }} · aktiv über {{ activePathLabel(binding.device_id) }}</div>
          </div>
          <div class="field">
            <span class="lbl">Verbindungsweg</span>
            <select v-model="settings[pathPolicyKey(binding.device_id)]" @change="onPathPolicyChange(binding.device_id)">
              <option value="auto">Automatisch (empfohlen)</option>
              <option value="relay_only">Muss über Relay</option>
              <option value="direct_only">Nur direkt</option>
              <option v-if="settings[pathPolicyKey(binding.device_id)] === 'prefer_direct'" value="prefer_direct">Direkt bevorzugen (alt)</option>
              <option v-if="settings[pathPolicyKey(binding.device_id)] === 'prefer_relay'" value="prefer_relay">Relay bevorzugen (alt)</option>
            </select>
          </div>
        </div>

        <div v-if="legacyRelayHost(binding.device_id)" class="legacy-path">
          Legacy-Relay aktiv · {{ legacyRelayHost(binding.device_id) }}:{{ legacyRelayPort(binding.device_id) }}
        </div>

        <div v-if="relayIds.length && settings[pathPolicyKey(binding.device_id)] !== 'direct_only'" class="relay-endpoints">
          <div v-for="relayId in relayIds" :key="`${binding.device_id}-${relayId}`" class="endpoint-summary">
            <span class="endpoint-state" :class="relayEndpointStateClass(binding.device_id, relayId)">{{ relayEndpointStateLabel(binding.device_id, relayId) }}</span>
            <span class="mono-mute">{{ relayName(relayId) }} · Port {{ endpointPortLabel(binding.device_id, relayId) }}</span>
          </div>
        </div>

        <details v-if="relayIds.length" class="advanced">
          <summary>Feinjustage</summary>
          <div class="relay-endpoints" style="margin-top: 12px;">
            <div v-for="relayId in relayIds" :key="`adv-${binding.device_id}-${relayId}`" class="relay-endpoint-row">
              <span>{{ relayName(relayId) }}</span>
              <input v-model="settings[relayEndpointKey(binding.device_id, relayId, 'port')]" class="compact-input" :placeholder="`Port · auto ${autoPortFor(binding, relayId)}`" />
              <input v-model="settings[relayEndpointKey(binding.device_id, relayId, 'host')]" class="compact-input" :placeholder="relayHost(relayId) || 'go2rtc-Host'" />
              <input v-model="settings[relayEndpointKey(binding.device_id, relayId, 'target_host')]" class="compact-input" :placeholder="binding.device?.last_ip || 'Ziel-IP'" />
              <input v-model="settings[relayEndpointKey(binding.device_id, relayId, 'target_port')]" class="compact-input" placeholder="554" />
            </div>
          </div>
          <div class="btn-row" style="margin-top: 12px;">
            <button class="btn sm primary" type="button" @click="saveEndpointOverrides(binding.device_id)">Speichern</button>
            <span class="mono-mute" style="font-size: 11px;">Leere Felder = automatische Werte (Port aus Kameraplatz, Ziel = Kamera-IP).</span>
          </div>
        </details>
      </div>
    </div>
  </section>

  <div v-if="showRelayModal" class="modal-backdrop" @click.self="closeRelayModal">
    <form class="modal" @submit.prevent="onAddRelay">
      <div class="modal-head">
        <div><div class="eyebrow">Netzwerk & Relay</div><h2>Relay hinzufügen</h2></div>
        <button class="btn icon sm ghost" type="button" title="Schließen" @click="closeRelayModal">×</button>
      </div>
      <div class="split">
        <div class="field"><span class="lbl">Name</span><input v-model="relayDraft.name" placeholder="NAS Relay" autofocus /></div>
        <div class="field"><span class="lbl">SSH-Ziel</span><input v-model="relayDraft.sshTarget" placeholder="nas oder user@nas" /></div>
        <div class="field"><span class="lbl">Host aus go2rtc-Docker</span><input v-model="relayDraft.host" placeholder="host.docker.internal" /></div>
      </div>
      <div class="modal-foot">
        <span class="mono-mute">Mehr ist nicht nötig — Ports und Kamera-Ziele werden automatisch vergeben.</span>
        <div class="btn-row">
          <button class="btn ghost" type="button" @click="closeRelayModal">Abbrechen</button>
          <button class="btn primary" type="submit" :disabled="savingRelay || !relayDraft.name || !relayDraft.sshTarget">{{ savingRelay ? 'Speichert…' : 'Hinzufügen & starten' }}</button>
        </div>
      </div>
    </form>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useSystem } from '../../composables/useSystem'
import type { Binding } from '../../types'

const {
  settings, relayIds, cameraBindings, error,
  loadAll, refreshStatus, saveSettings, addRelay, removeRelay, relayAction, sanitizeID,
  relaySettingKey, relayEndpointKey, pathPolicyKey, relayName, relayHost, relayAutoStart, relayStatusFor,
  relayStateLabel, relayStateClass, relayEndpointStatus, relayEndpointStateLabel, relayEndpointStateClass,
  legacyRelayHost, legacyRelayPort
} = useSystem()

// Mirrors the backend's auto port scheme (paths.go): relay n → base 18554+20n, slot m → +m-1.
const relayPortBaseDefault = 18554
const relayPortBaseSpacing = 20

const relayDraft = reactive({ name: '', host: 'host.docker.internal', sshTarget: '' })
const relayActionBusy = ref('')
const showRelayModal = ref(false)
const savingRelay = ref(false)

function relayPortBaseFallback(relayId: string) {
  const index = Math.max(0, relayIds.value.indexOf(relayId))
  return relayPortBaseDefault + relayPortBaseSpacing * index
}

function autoPortFor(binding: Binding, relayId: string) {
  const slotNumber = Number((binding.slot_id || '').replace(/^\D+/, ''))
  if (!Number.isFinite(slotNumber) || slotNumber <= 0) return '—'
  const base = Number(settings[relaySettingKey(relayId, 'port_base')]) || relayPortBaseFallback(relayId)
  return String(base + slotNumber - 1)
}

function endpointPortLabel(deviceId: string, relayId: string) {
  return relayEndpointStatus(deviceId, relayId)?.local_port
    || settings[relayEndpointKey(deviceId, relayId, 'port')]
    || '—'
}

function activePathLabel(deviceId: string) {
  const kind = settings[`camera.active_path.${deviceId}.kind`]
  if (kind === 'relay') {
    const relayId = settings[`camera.active_path.${deviceId}.relay_id`]
    return relayId && relayId !== 'manual' ? `Relay ${relayName(relayId)}` : 'Relay'
  }
  if (kind === 'direct') return 'Direkt'
  return '—'
}

function openRelayModal() {
  relayDraft.name = relayIds.value.length ? '' : 'NAS Relay'
  relayDraft.sshTarget = ''
  relayDraft.host = 'host.docker.internal'
  showRelayModal.value = true
}
function closeRelayModal() {
  if (!savingRelay.value) showRelayModal.value = false
}

async function onAddRelay() {
  savingRelay.value = true
  error.value = ''
  try {
    const id = addRelay({ id: sanitizeID(relayDraft.name), name: relayDraft.name, host: relayDraft.host, sshTarget: relayDraft.sshTarget })
    if (!id) return
    await saveSettings()
    showRelayModal.value = false
    await relayAction(id, 'start').catch((err) => {
      error.value = err instanceof Error ? err.message : 'Relay konnte nicht gestartet werden.'
    })
  } finally {
    savingRelay.value = false
  }
}

async function onRemoveRelay(relayId: string) {
  removeRelay(relayId)
  await saveSettings()
  await refreshStatus()
}

async function onRelayAction(id: string, action: 'start' | 'stop' | 'restart') {
  relayActionBusy.value = `${action}:${id}`
  try {
    await relayAction(id, action)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Relay-Aktion fehlgeschlagen.'
  } finally {
    relayActionBusy.value = ''
  }
}

async function onAutoStartChange(relayId: string, e: Event) {
  settings[relaySettingKey(relayId, 'auto_start')] = (e.target as HTMLInputElement).checked ? 'true' : 'false'
  await saveSettings([relaySettingKey(relayId, 'auto_start')])
  await refreshStatus()
}

async function saveRelayConfig(relayId: string) {
  await saveSettings(['name', 'ssh_target', 'host', 'bind_host', 'port_base'].map((field) => relaySettingKey(relayId, field)))
  await onRelayAction(relayId, 'restart')
}

async function onPathPolicyChange(deviceId: string) {
  await saveSettings([pathPolicyKey(deviceId)])
  await refreshStatus()
}

async function saveEndpointOverrides(deviceId: string) {
  const keys = relayIds.value.flatMap((relayId) =>
    ['port', 'host', 'target_host', 'target_port'].map((field) => relayEndpointKey(deviceId, relayId, field))
  )
  await saveSettings(keys)
  await refreshStatus()
}

onMounted(() => void loadAll())
</script>

<style scoped>
.endpoint-summary {
  display: flex;
  align-items: center;
  gap: 10px;
}
</style>
