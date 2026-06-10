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
import { useRouter } from 'vue-router'
import { useSystem } from '../../composables/useSystem'

const router = useRouter()
const {
  settings, relayIds, error,
  loadAll, saveSettings, addRelay, relayAction, sanitizeID,
  relaySettingKey, relayName, relayStatusFor, relayStateLabel, relayStateClass
} = useSystem()

const relayDraft = reactive({ name: '', host: 'host.docker.internal', sshTarget: '' })
const showRelayModal = ref(false)
const savingRelay = ref(false)

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
