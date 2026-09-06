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
          <button class="btn sm danger" type="button" @click="confirmRemove=true">Entfernen</button>
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


    </section>

    <SettingsForm ref="configEditor" title="Konfiguration" :setting-keys="configKeys" :after-save="() => onAction('restart')">
      <template #summary><dl class="spec"><div><dt>Name</dt><dd>{{ relayName(relayId) }}</dd></div><div><dt>SSH-Ziel</dt><dd>{{ settings[relaySettingKey(relayId,'ssh_target')] }}</dd></div><div><dt>go2rtc-Host</dt><dd>{{ settings[relaySettingKey(relayId,'host')] }}</dd></div><div><dt>Automatisch aktiv halten</dt><dd>{{ relayAutoStart(relayId) ? 'Ja' : 'Nein' }}</dd></div></dl></template>
      <label class="toggle-row compact">
        <input type="checkbox" :checked="relayAutoStart(relayId)" @change="setBool(relaySettingKey(relayId, 'auto_start'), $event)" />
        <div><div class="lbl-main">Automatisch aktiv halten</div><div class="lbl-sub">Watchdog startet das Relay bei Ausfall erneut (empfohlen).</div></div>
      </label>
      <div class="relay-config-grid">
        <div class="field"><span class="lbl">Name</span><input aria-label="Name" v-model="settings[relaySettingKey(relayId, 'name')]" class="compact-input" :placeholder="relayId" /></div>
        <div class="field"><span class="lbl">SSH-Ziel</span><input aria-label="SSH-Ziel" v-model="settings[relaySettingKey(relayId, 'ssh_target')]" class="compact-input" placeholder="nas oder user@nas" /></div>
        <div class="field"><span class="lbl">Host aus go2rtc-Docker</span><input aria-label="Host aus go2rtc-Docker" v-model="settings[relaySettingKey(relayId, 'host')]" class="compact-input" placeholder="host.docker.internal" /></div>
        <div class="field"><span class="lbl">Bind-Adresse</span><input aria-label="Bind-Adresse" v-model="settings[relaySettingKey(relayId, 'bind_host')]" class="compact-input" placeholder="127.0.0.1" /></div>
        <div class="field"><span class="lbl">Port-Basis</span><input aria-label="Port-Basis" v-model="settings[relaySettingKey(relayId, 'port_base')]" class="compact-input" :placeholder="String(relayPortBaseFallback)" /></div>
      </div>
      <p class="mono-mute">Speichern startet das Relay neu, damit die Konfiguration wirksam wird.</p>
    </SettingsForm>
    <button class="mobile-fab" aria-label="Relay bearbeiten" @click="configEditor?.edit()"><AppIcon name="edit" /></button>
    <AdminDialog :open="confirmRemove" title="Relay entfernen?" compact :busy="busy !== ''" @close="confirmRemove=false"><p>„{{ relayName(relayId) }}“ wird aus der Konfiguration entfernt. Kameras können diesen Ersatzpfad anschließend nicht mehr nutzen.</p><div class="form-actions"><button class="btn" @click="confirmRemove=false">Abbrechen</button><button class="btn danger" @click="onRemove">Entfernen</button></div></AdminDialog>

    <section class="panel">
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
import AppIcon from '../../components/AppIcon.vue'
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import SettingsForm from '../../components/SettingsForm.vue'
import AdminDialog from '../../components/AdminDialog.vue'
import { useSystem } from '../../composables/useSystem'

const route = useRoute()
const router = useRouter()
const {
  settings, relayIds, error, setBool,
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
const configEditor=ref<InstanceType<typeof SettingsForm>>(),confirmRemove=ref(false)
const configKeys=computed(()=>['name','ssh_target','host','bind_host','port_base','auto_start'].map(field=>relaySettingKey(relayId.value,field)))

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

async function onRemove() {
  confirmRemove.value=false
  removeRelay(relayId.value)
  if (!await saveSettings(['camera.relay.ids'])) return
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
