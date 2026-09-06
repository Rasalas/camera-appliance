<template>
  <section class="panel">
    <div class="panel-head">
      <h2>Relays</h2>
      <div class="device-head-actions">
        <div class="right">{{ relayIds.length ? `${relayIds.length} eingerichtet` : 'nicht eingerichtet' }}</div>
        <button class="btn primary desktop-primary" type="button" @click="openRelayModal"><AppIcon name="plus" />Relay hinzufügen</button>
      </div>
    </div>

    <div class="mono-mute">
      Ein Relay leitet Kamera-Streams per SSH über einen anderen Host. Einmal eingerichtet, steht es jeder Kamera
      automatisch als Ersatzpfad zur Verfügung — der Watchdog wechselt bei Verbindungsproblemen von selbst.
      Ob eine Kamera das Relay nutzen <i>muss</i>, legst du auf ihrer Detailseite unter „Verbindung“ fest.
    </div>

    <div v-if="!relayIds.length" class="empty">Kein Relay eingerichtet. Direkt erreichbare Kameras funktionieren auch ohne.</div>
    <div v-else class="relay-list">
      <RouterLink v-for="relayId in relayIds" :key="relayId" :to="`/system/relays/${relayId}`" class="relay-row">
        <span class="state-dot" :class="relayStateClass(relayId)"></span>
        <div>
          <div class="name">{{ relayName(relayId) }}</div>
          <div class="mono-mute sub">SSH {{ settings[relaySettingKey(relayId, 'ssh_target')] || relayId }}</div>
        </div>
        <div class="state">{{ relayStateLabel(relayId) }}</div>
        <div class="mono-mute">{{ forwardSummary(relayId) }}</div>
        <span class="chevron">›</span>
      </RouterLink>
    </div>
  </section>

  <button class="mobile-fab" aria-label="Relay hinzufügen" @click="openRelayModal"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 4v16M4 12h16"/></svg></button>
  <AdminDialog ref="relayDialog" :open="showRelayModal" title="Relay hinzufügen" :dirty="relayDirty" :busy="savingRelay" @close="showRelayModal=false">
    <form @submit.prevent="onAddRelay">
      <div class="split">
        <label class="field"><span class="lbl">Name</span><input aria-label="Name" v-model="relayDraft.name" placeholder="NAS Relay" required autofocus /></label>
        <label class="field"><span class="lbl">SSH-Ziel</span><input aria-label="SSH-Ziel" v-model="relayDraft.sshTarget" required placeholder="nas oder user@nas" /></label>
        <label class="field"><span class="lbl">Host aus go2rtc-Docker</span><input aria-label="Host aus go2rtc-Docker" v-model="relayDraft.host" placeholder="host.docker.internal" /></label>
      </div>
      <p v-if="error" class="notice err" role="alert">{{ error }}</p>
      <div class="modal-foot">
        <span class="mono-mute">Mehr ist nicht nötig — Ports und Kamera-Ziele werden automatisch vergeben.</span>
        <div class="btn-row">
          <button class="btn ghost" type="button" @click="closeRelayModal">Abbrechen</button>
          <button class="btn primary" type="submit" :disabled="savingRelay || !relayDraft.name || !relayDraft.sshTarget"><AppIcon name="plus" />{{ savingRelay ? 'Speichert…' : 'Hinzufügen & starten' }}</button>
        </div>
      </div>
    </form>
  </AdminDialog>
</template>

<script setup lang="ts">
import AppIcon from '../../components/AppIcon.vue'
import { computed, onMounted, reactive, ref } from 'vue'
import AdminDialog from '../../components/AdminDialog.vue'
import { useDraftGuard } from '../../composables/discardChanges'
import { useRouter } from 'vue-router'
import { useSystem } from '../../composables/useSystem'
import { relaySettingKeys } from '../../composables/settingsDraft'

const router = useRouter()
const {
  settings, relayIds, error,
  loadAll, saveSettings, addRelay, relayAction, sanitizeID,
  relaySettingKey, relayName, relayStatusFor, relayStateLabel, relayStateClass
} = useSystem()

const relayDraft = reactive({ name: '', host: 'host.docker.internal', sshTarget: '' })
const showRelayModal = ref(false)
const savingRelay = ref(false)
const relayDialog=ref<InstanceType<typeof AdminDialog>>(), relayBaseline=ref('')
const relayDirty=computed(()=>showRelayModal.value && JSON.stringify(relayDraft)!==relayBaseline.value)
useDraftGuard(()=>relayDirty.value,()=>{showRelayModal.value=false})

function forwardSummary(relayId: string) {
  const endpoints = relayStatusFor(relayId)?.endpoints || []
  if (!endpoints.length) return 'keine Weiterleitungen'
  const ok = endpoints.filter((endpoint) => endpoint.state === 'ok').length
  return `${ok}/${endpoints.length} Weiterleitungen ok`
}

function openRelayModal() {
  relayDraft.name = relayIds.value.length ? '' : 'NAS Relay'
  relayDraft.sshTarget = ''
  relayDraft.host = 'host.docker.internal'
  relayBaseline.value=JSON.stringify(relayDraft)
  showRelayModal.value = true
}
function closeRelayModal() {
  void relayDialog.value?.requestClose()
}

async function onAddRelay() {
  savingRelay.value = true
  error.value = ''
  try {
    const id = addRelay({ id: sanitizeID(relayDraft.name), name: relayDraft.name, host: relayDraft.host, sshTarget: relayDraft.sshTarget })
    if (!id) return
    if (!await saveSettings(['camera.relay.ids', ...relaySettingKeys(id)])) return
    showRelayModal.value = false
    await relayAction(id, 'start').catch((err) => {
      error.value = err instanceof Error ? err.message : 'Relay konnte nicht gestartet werden.'
    })
    await router.push(`/system/relays/${id}`)
  } finally {
    savingRelay.value = false
  }
}

onMounted(() => void loadAll())
</script>

<style scoped>
.relay-list { display: grid; gap: 8px; }
.relay-row {
  display: grid;
  grid-template-columns: 12px minmax(180px, 1.4fr) minmax(90px, .8fr) minmax(140px, 1fr) auto;
  gap: 14px;
  align-items: center;
  padding: 14px;
  background: var(--surface);
  border-radius: var(--radius-sm);
  transition: background .12s ease;
}
.relay-row:hover { background: var(--raised); }
.relay-row .name { color: var(--ink); font-size: 13px; }
.relay-row .sub { font-size: 11px; margin-top: 2px; }
.relay-row .state {
  font-size: 10.5px;
  text-transform: uppercase;
  letter-spacing: .12em;
  color: var(--ink-soft);
}
.relay-row .chevron { color: var(--ink-dim); font-size: 16px; }
.relay-row .state-dot.ok { background: var(--live); box-shadow: 0 0 8px var(--live); }
.relay-row .state-dot.warn { background: var(--warn); }
.relay-row .state-dot.err { background: var(--danger); }
@media (max-width: 820px) {
  .relay-row { grid-template-columns: 12px 1fr auto; }
  .relay-row .state, .relay-row .chevron { display: none; }
}
</style>
