<template>
  <section class="panel card">
    <div class="panel-head">
      <h2>Kamera-Identitäten</h2>
      <div class="device-head-actions">
        <div class="right">{{ credentialIdentities.length }} gespeichert</div>
        <button class="btn sm primary" type="button" @click="openNewIdentity">Identität hinzufügen</button>
      </div>
    </div>
    <div class="mono-mute">Identitäten sind wiederverwendbare Kamera-Logins. Beim Bildtest werden sie automatisch auf Kameras ohne eigenes Passwort ausprobiert; die Stream-Auswahl bleibt an der Kamera.</div>
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
  credentialIdentities, error,
  loadAll, saveCredentialIdentity, deleteCredentialIdentity, passwordSourceLabel
} = useSystem()

const showIdentityModal = ref(false)
const savingIdentity = ref(false)
const identityForm = reactive({ id: '', name: '', username: '', password: '' })

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
