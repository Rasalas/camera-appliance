<template>
  <section class="panel card">
    <div class="panel-head">
      <h2>Relays und Pfade</h2>
      <div class="device-head-actions">
        <div class="right">{{ relayIds.length }} Relay{{ relayIds.length === 1 ? '' : 's' }}</div>
        <button class="btn sm primary" type="button" @click="onAddRelay">Relay hinzufügen</button>
      </div>
    </div>

    <div class="split">
      <div class="field"><span class="lbl">Relay-ID</span><input v-model="relayDraft.id" placeholder="nas" /></div>
      <div class="field"><span class="lbl">Name</span><input v-model="relayDraft.name" placeholder="NAS Relay" /></div>
      <div class="field"><span class="lbl">Host aus go2rtc-Docker</span><input v-model="relayDraft.host" placeholder="host.docker.internal" /></div>
      <div class="field"><span class="lbl">SSH-Ziel</span><input v-model="relayDraft.sshTarget" placeholder="nas oder user@nas" /></div>
    </div>

    <div v-if="!relayIds.length" class="empty">Noch keine Relays definiert. Legacy-Overrides werden weiter unterstützt.</div>
    <div v-else class="relay-config-list">
      <div v-for="relayId in relayIds" :key="relayId" class="relay-config">
        <div class="relay-config-head">
          <div>
            <div class="slot">Relay</div>
            <div class="name">{{ relayName(relayId) }}</div>
            <div class="mono-mute">{{ relayId }}</div>
          </div>
          <div class="btn-row">
            <button class="btn sm" type="button" :disabled="relayActionBusy === `start:${relayId}`" @click="onRelayAction(relayId, 'start')">{{ relayActionBusy === `start:${relayId}` ? 'Startet…' : 'Start' }}</button>
            <button class="btn sm ghost" type="button" :disabled="relayActionBusy === `stop:${relayId}`" @click="onRelayAction(relayId, 'stop')">{{ relayActionBusy === `stop:${relayId}` ? 'Stoppt…' : 'Stop' }}</button>
            <button class="btn sm ghost" type="button" :disabled="relayActionBusy === `restart:${relayId}`" @click="onRelayAction(relayId, 'restart')">{{ relayActionBusy === `restart:${relayId}` ? 'Startet…' : 'Restart' }}</button>
            <button class="btn sm danger" type="button" @click="removeRelay(relayId)">Entfernen</button>
          </div>
        </div>

        <div class="relay-config-grid">
          <div class="field"><span class="lbl">Name</span><input v-model="settings[relaySettingKey(relayId, 'name')]" class="compact-input" :placeholder="relayId" /></div>
          <div class="field">
            <span class="lbl">Typ</span>
            <select v-model="settings[relaySettingKey(relayId, 'type')]"><option value="ssh_local_forward">SSH Local Forward</option></select>
          </div>
          <div class="field"><span class="lbl">go2rtc-Host</span><input v-model="settings[relaySettingKey(relayId, 'host')]" class="compact-input" placeholder="host.docker.internal" /></div>
          <div class="field"><span class="lbl">SSH-Ziel</span><input v-model="settings[relaySettingKey(relayId, 'ssh_target')]" class="compact-input" placeholder="nas oder user@nas" /></div>
          <div class="field"><span class="lbl">Bind-Adresse</span><input v-model="settings[relaySettingKey(relayId, 'bind_host')]" class="compact-input" placeholder="127.0.0.1" /></div>
          <label class="toggle-row relay-auto">
            <input type="checkbox" :checked="relayAutoStart(relayId)" @change="setBool(relaySettingKey(relayId, 'auto_start'), $event)" />
            <div><div class="lbl-main">Auto-Start</div><div class="lbl-sub">Watchdog startet diesen Relay bei Ausfall erneut.</div></div>
          </label>
        </div>

        <div class="relay-runtime" :class="relayStateClass(relayId)">
          <span class="state-dot"></span>
          <div>
            <div class="lbl-main">{{ relayStateLabel(relayId) }}</div>
            <div class="lbl-sub">{{ relayStatusFor(relayId)?.message || 'Noch kein Status.' }}</div>
            <div v-if="relayStatusFor(relayId)?.last_error" class="mono-mute">Fehler · {{ relayStatusFor(relayId)?.last_error }}</div>
          </div>
          <div class="mono-mute">{{ relayStatusFor(relayId)?.pid ? `PID ${relayStatusFor(relayId)?.pid}` : relayStatusFor(relayId)?.log_path || '' }}</div>
        </div>
      </div>
    </div>

    <div class="panel-subhead">
      <h3>Kamera-Pfade</h3>
      <div class="right">Auto versucht den letzten funktionierenden Pfad, dann direkt und Relays.</div>
    </div>

    <div v-if="!cameraBindings.length" class="empty">Noch keine Kameras aktiviert.</div>
    <div v-else class="relay-camera-list">
      <div v-for="binding in cameraBindings" :key="binding.device_id" class="relay-camera">
        <div class="relay-camera-main">
          <div>
            <div class="name">{{ binding.label || binding.slot?.label || binding.slot_id }}</div>
            <div class="mono-mute">{{ binding.device?.last_ip || 'keine IP' }} · {{ binding.stream_name || 'stream2' }}</div>
          </div>
          <select v-model="settings[pathPolicyKey(binding.device_id)]">
            <option value="auto">Auto</option>
            <option value="prefer_direct">Direkt bevorzugen</option>
            <option value="prefer_relay">Relay bevorzugen</option>
            <option value="direct_only">Nur direkt</option>
            <option value="relay_only">Nur Relay</option>
          </select>
        </div>

        <div v-if="legacyRelayHost(binding.device_id)" class="legacy-path">
          Legacy-Relay aktiv · {{ legacyRelayHost(binding.device_id) }}:{{ legacyRelayPort(binding.device_id) }}
        </div>

        <div v-if="relayIds.length" class="relay-endpoints">
          <div v-for="relayId in relayIds" :key="`${binding.device_id}-${relayId}`" class="relay-endpoint-row">
            <span>{{ relayName(relayId) }}</span>
            <input v-model="settings[relayEndpointKey(binding.device_id, relayId, 'host')]" :placeholder="relayHost(relayId) || 'Host'" />
            <input v-model="settings[relayEndpointKey(binding.device_id, relayId, 'port')]" placeholder="Lokaler Port" />
            <input v-model="settings[relayEndpointKey(binding.device_id, relayId, 'target_host')]" :placeholder="binding.device?.last_ip || 'Ziel-IP'" />
            <input v-model="settings[relayEndpointKey(binding.device_id, relayId, 'target_port')]" placeholder="554" />
            <span class="endpoint-state" :class="relayEndpointStateClass(binding.device_id, relayId)">{{ relayEndpointStateLabel(binding.device_id, relayId) }}</span>
          </div>
        </div>
      </div>
    </div>

    <div class="btn-row" style="margin-top: 8px;">
      <button class="btn primary" @click="saveSettings()">Pfade speichern</button>
    </div>
  </section>

  <section class="panel card">
    <div class="panel-head">
      <h2>Kamera-Identitäten</h2>
      <div class="device-head-actions">
        <div class="right">{{ credentialIdentities.length }} gespeichert</div>
        <button class="btn icon sm" type="button" title="Identität hinzufügen" @click="openNewIdentity">+</button>
      </div>
    </div>
    <div class="mono-mute">Identitäten sind wiederverwendbare Logins. Stream-Auswahl bleibt an Kamera, Zuordnung oder Bildtest.</div>
    <div v-if="!credentialIdentities.length" class="empty">Noch keine Identitäten gespeichert.</div>
    <div v-else class="result-list">
      <div v-for="identity in credentialIdentities" :key="identity.id" class="result-row ok">
        <span class="slot">Login</span>
        <span class="name">{{ identity.name }}</span>
        <span class="ip">{{ identity.username }}</span>
        <span class="stream">{{ identity.password_set ? passwordSourceLabel(identity.password_source) : 'kein Passwort' }}</span>
        <button class="btn sm ghost" type="button" @click="editIdentity(identity)">Bearbeiten</button>
        <button class="btn sm danger" type="button" @click="onDeleteIdentity(identity.id)">Entfernen</button>
      </div>
    </div>
  </section>

  <div v-if="showIdentityModal" class="modal-backdrop" @click.self="closeIdentityModal">
    <form class="modal" @submit.prevent="onSaveIdentity">
      <div class="modal-head">
        <div><div class="eyebrow">Kamera-Identitäten</div><h2>{{ identityForm.id ? 'Identität bearbeiten' : 'Identität hinzufügen' }}</h2></div>
        <button class="btn icon sm ghost" type="button" title="Schließen" @click="closeIdentityModal">×</button>
      </div>
      <div class="split">
        <div class="field"><span class="lbl">Name</span><input v-model="identityForm.name" placeholder="Tapo Außenkameras" autofocus /></div>
        <div class="field"><span class="lbl">Benutzername</span><input v-model="identityForm.username" placeholder="Kamera-Benutzer" /></div>
        <div class="field"><span class="lbl">Passwort</span><input v-model="identityForm.password" type="password" :placeholder="identityForm.id ? 'leer lassen, um Passwort zu behalten' : 'Kamera-Passwort'" /></div>
      </div>
      <div class="modal-foot">
        <span class="mono-mute">Wird beim Bildtest auf passende Kameras ausprobiert.</span>
        <div class="btn-row">
          <button class="btn ghost" type="button" @click="closeIdentityModal">Abbrechen</button>
          <button class="btn primary" type="submit" :disabled="savingIdentity || !identityForm.name || !identityForm.username">{{ savingIdentity ? 'Speichert…' : 'Speichern' }}</button>
        </div>
      </div>
    </form>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useSystem } from '../../composables/useSystem'
import type { CredentialIdentity } from '../../types'

const {
  settings, relayIds, cameraBindings, credentialIdentities, error,
  loadAll, saveSettings, addRelay, removeRelay, relayAction, saveCredentialIdentity, deleteCredentialIdentity,
  setBool, relaySettingKey, relayEndpointKey, pathPolicyKey, relayName, relayHost, relayAutoStart, relayStatusFor,
  relayStateLabel, relayStateClass, relayEndpointStateLabel, relayEndpointStateClass, legacyRelayHost, legacyRelayPort, passwordSourceLabel
} = useSystem()

const relayDraft = reactive({ id: 'nas', name: 'NAS Relay', host: 'host.docker.internal', sshTarget: 'nas' })
const relayActionBusy = ref('')
const showIdentityModal = ref(false)
const savingIdentity = ref(false)
const identityForm = reactive({ id: '', name: '', username: '', password: '' })

function onAddRelay() {
  const id = addRelay(relayDraft)
  if (!id) return
  relayDraft.id = ''
  relayDraft.name = ''
  relayDraft.host = ''
  relayDraft.sshTarget = ''
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

function openNewIdentity() {
  identityForm.id = ''
  identityForm.name = ''
  identityForm.username = ''
  identityForm.password = ''
  showIdentityModal.value = true
}
function editIdentity(identity: CredentialIdentity) {
  identityForm.id = identity.id
  identityForm.name = identity.name
  identityForm.username = identity.username
  identityForm.password = ''
  showIdentityModal.value = true
}
function closeIdentityModal() {
  if (!savingIdentity.value) showIdentityModal.value = false
}
async function onSaveIdentity() {
  savingIdentity.value = true
  try {
    await saveCredentialIdentity(identityForm)
    showIdentityModal.value = false
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Identität konnte nicht gespeichert werden.'
  } finally {
    savingIdentity.value = false
  }
}
async function onDeleteIdentity(id: string) {
  try {
    await deleteCredentialIdentity(id)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Identität konnte nicht entfernt werden.'
  }
}

onMounted(() => void loadAll())
</script>
