<template>
  <section class="panel card">
    <div class="panel-head">
      <h2>Relays</h2>
      <div class="device-head-actions">
        <div class="right">{{ relayIds.length ? `${relayIds.length} eingerichtet` : 'nicht eingerichtet' }}</div>
        <button class="btn sm primary" type="button" @click="openRelayModal">Relay hinzufügen</button>
      </div>
    </div>

    <div class="mono-mute">
      Ein Relay leitet Kamera-Streams per SSH über einen anderen Host. Einmal eingerichtet, steht es jeder Kamera
      automatisch als Ersatzpfad zur Verfügung — die Ports werden pro Kameraplatz automatisch vergeben, und der
      Watchdog wechselt bei Verbindungsproblemen von selbst auf das Relay und wieder zurück.
      Ob eine Kamera das Relay nutzen <i>muss</i>, legst du auf ihrer Detailseite unter „Verbindung“ fest.
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

        <div v-if="relayStatusFor(relayId)?.endpoints?.length" class="relay-endpoints">
          <div v-for="endpoint in relayStatusFor(relayId)?.endpoints" :key="endpoint.device_id" class="endpoint-summary">
            <span class="endpoint-state" :class="endpointStateClass(endpoint.state)">{{ endpointStateLabel(endpoint.state) }}</span>
            <span class="mono-mute">{{ endpoint.label || endpoint.device_id }} · {{ endpoint.slot_id || '—' }} · Port {{ endpoint.local_port || '—' }} → {{ endpoint.target_host || '?' }}:{{ endpoint.target_port }}</span>
          </div>
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

  <div v-if="showRelayModal" class="modal-backdrop" @click.self="closeRelayModal">
    <form class="modal" @submit.prevent="onAddRelay">
      <div class="modal-head">
        <div><div class="eyebrow">Relays</div><h2>Relay hinzufügen</h2></div>
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

const {
  settings, relayIds, error,
  loadAll, refreshStatus, saveSettings, addRelay, removeRelay, relayAction, sanitizeID,
  relaySettingKey, relayName, relayAutoStart, relayStatusFor, relayStateLabel, relayStateClass
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

function endpointStateLabel(state: string) {
  if (state === 'ok') return 'OK'
  if (state === 'failed') return 'Offline'
  return 'Unvollständig'
}
function endpointStateClass(state: string) {
  if (state === 'ok') return 'ok'
  if (state === 'failed') return 'err'
  return 'warn'
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

onMounted(() => void loadAll())
</script>
