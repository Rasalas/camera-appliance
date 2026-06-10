<template>
  <div class="mono-mute" style="font-size: 11px;">
    <RouterLink to="/system/relays" class="mono-mute">← Alle Relays</RouterLink>
  </div>

  <div v-if="missing" class="empty">Relay „{{ relayId }}“ ist nicht konfiguriert.</div>

  <template v-else>
    <section class="panel card">
      <div class="panel-head">
        <h2>{{ relayName(relayId) }}</h2>
        <div class="btn-row">
          <button v-if="relayStatusFor(relayId)?.process_state !== 'running'" class="btn sm" type="button" :disabled="busy === 'start'" @click="onAction('start')">{{ busy === 'start' ? 'Startet…' : 'Start' }}</button>
          <button v-else class="btn sm ghost" type="button" :disabled="busy === 'restart'" @click="onAction('restart')">{{ busy === 'restart' ? 'Startet…' : 'Restart' }}</button>
          <button class="btn sm ghost" type="button" :disabled="busy === 'stop'" @click="onAction('stop')">{{ busy === 'stop' ? 'Stoppt…' : 'Stop' }}</button>
          <button class="btn sm danger" type="button" @click="onRemove">Entfernen</button>
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
        <input type="checkbox" :checked="relayAutoStart(relayId)" @change="onAutoStartChange($event)" />
        <div><div class="lbl-main">Automatisch aktiv halten</div><div class="lbl-sub">Watchdog startet das Relay bei Ausfall erneut (empfohlen).</div></div>
      </label>
    </section>

    <section class="panel card">
      <div class="panel-head">
        <h2>Konfiguration</h2>
        <div class="right">{{ relayId }}</div>
      </div>
      <div class="relay-config-grid">
        <div class="field"><span class="lbl">Name</span><input v-model="settings[relaySettingKey(relayId, 'name')]" class="compact-input" :placeholder="relayId" /></div>
        <div class="field"><span class="lbl">SSH-Ziel</span><input v-model="settings[relaySettingKey(relayId, 'ssh_target')]" class="compact-input" placeholder="nas oder user@nas" /></div>
        <div class="field"><span class="lbl">Host aus go2rtc-Docker</span><input v-model="settings[relaySettingKey(relayId, 'host')]" class="compact-input" placeholder="host.docker.internal" /></div>
        <div class="field"><span class="lbl">Bind-Adresse</span><input v-model="settings[relaySettingKey(relayId, 'bind_host')]" class="compact-input" placeholder="127.0.0.1" /></div>
        <div class="field"><span class="lbl">Port-Basis</span><input v-model="settings[relaySettingKey(relayId, 'port_base')]" class="compact-input" :placeholder="String(relayPortBaseFallback)" /></div>
      </div>
      <div class="btn-row">
        <button class="btn sm primary" type="button" :disabled="busy !== ''" @click="onSaveConfig">Speichern & neu starten</button>
        <span class="mono-mute" style="font-size: 11px;">Log · {{ relayStatusFor(relayId)?.log_path || '—' }}</span>
      </div>
    </section>

    <section class="panel card">
      <div class="panel-head">
        <h2>Weiterleitungen</h2>
        <div class="right">automatisch aus den aktiven Kameras</div>
      </div>
      <div class="mono-mute">
        Pro Kameraplatz wird automatisch ein lokaler Port weitergeleitet. Verbindungsweg und
        Port-Anpassungen einer Kamera findest du auf ihrer Detailseite unter „Verbindung“.
      </div>
      <div v-if="!endpoints.length" class="empty">Keine Weiterleitungen — noch keine Kamera aktiviert.</div>
      <div v-else class="forward-list">
        <RouterLink v-for="endpoint in endpoints" :key="endpoint.device_id" :to="`/kamera/${endpoint.device_id}`" class="forward-row">
          <span class="endpoint-state" :class="endpointStateClass(endpoint.state)">{{ endpointStateLabel(endpoint.state) }}</span>
          <div>
            <div class="name">{{ endpoint.label || endpoint.device_id }}</div>
            <div class="mono-mute sub">{{ endpoint.slot_id || '—' }} · Port {{ endpoint.local_port || '—' }} → {{ endpoint.target_host || '?' }}:{{ endpoint.target_port }}</div>
          </div>
          <span class="mono-mute" style="font-size: 11px;">{{ endpoint.message }}</span>
          <span class="chevron">›</span>
        </RouterLink>
      </div>
    </section>
  </template>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useSystem } from '../../composables/useSystem'

const route = useRoute()
const router = useRouter()
const {
  settings, relayIds, error,
  loadAll, refreshStatus, saveSettings, removeRelay, relayAction,
  relaySettingKey, relayName, relayAutoStart, relayStatusFor, relayStateLabel, relayStateClass
} = useSystem()

// Mirrors the backend's auto port scheme (paths.go): relay n → base 18554+20n, slot m → +m-1.
const relayPortBaseDefault = 18554
const relayPortBaseSpacing = 20

const relayId = computed(() => String(route.params.id))
const loadedOnce = ref(false)
const missing = computed(() => loadedOnce.value && !relayIds.value.includes(relayId.value))
const endpoints = computed(() => relayStatusFor(relayId.value)?.endpoints || [])
const relayPortBaseFallback = computed(() => {
  const index = Math.max(0, relayIds.value.indexOf(relayId.value))
  return relayPortBaseDefault + relayPortBaseSpacing * index
})
const busy = ref('')

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

async function onAction(action: 'start' | 'stop' | 'restart') {
  busy.value = action
  try {
    await relayAction(relayId.value, action)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Relay-Aktion fehlgeschlagen.'
  } finally {
    busy.value = ''
  }
}

async function onAutoStartChange(e: Event) {
  settings[relaySettingKey(relayId.value, 'auto_start')] = (e.target as HTMLInputElement).checked ? 'true' : 'false'
  await saveSettings([relaySettingKey(relayId.value, 'auto_start')])
  await refreshStatus()
}

async function onSaveConfig() {
  await saveSettings(['name', 'ssh_target', 'host', 'bind_host', 'port_base'].map((field) => relaySettingKey(relayId.value, field)))
  await onAction('restart')
}

async function onRemove() {
  removeRelay(relayId.value)
  await saveSettings()
  await refreshStatus()
  await router.push('/system/relays')
}

onMounted(async () => {
  await loadAll()
  await refreshStatus()
  loadedOnce.value = true
})
</script>

<style scoped>
.forward-list { display: grid; gap: 8px; }
.forward-row {
  display: grid;
  grid-template-columns: 90px minmax(180px, 1.2fr) minmax(160px, 1fr) auto;
  gap: 14px;
  align-items: center;
  padding: 12px 14px;
  background: var(--bg);
  border-radius: var(--radius-sm);
  transition: background .12s ease;
}
.forward-row:hover { background: var(--raised); }
.forward-row .name { color: var(--ink); font-size: 12.5px; }
.forward-row .sub { font-size: 11px; margin-top: 2px; }
.forward-row .chevron { color: var(--ink-dim); font-size: 16px; }
@media (max-width: 820px) {
  .forward-row { grid-template-columns: 90px 1fr auto; }
  .forward-row > .mono-mute { display: none; }
}
</style>
