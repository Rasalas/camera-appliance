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
      <div
        v-for="identity in credentialIdentities"
        :key="identity.id"
        class="result-row identity-row ok"
        role="button"
        tabindex="0"
        @click="editIdentity(identity)"
        @keydown.enter.prevent="editIdentity(identity)"
        @keydown.space.prevent="editIdentity(identity)"
      >
        <span class="slot">Login</span>
        <span class="name">{{ identity.name }}</span>
        <span class="ip">{{ identity.username }}</span>
        <span class="stream">{{ identity.password_set ? passwordSourceLabel(identity.password_source) : 'kein Passwort' }}</span>
        <span class="identity-actions">
          <button class="btn sm ghost" type="button" :disabled="savingIdentity" @click.stop="duplicateIdentity(identity)">Duplizieren</button>
          <button class="btn icon sm ghost" type="button" title="Bearbeiten" aria-label="Bearbeiten" @click.stop="editIdentity(identity)">✎</button>
          <button class="btn icon sm danger" type="button" title="Entfernen" aria-label="Entfernen" @click.stop="askDeleteIdentity(identity)">
            <svg aria-hidden="true" viewBox="0 0 24 24" class="icon-svg"><path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3" /></svg>
          </button>
        </span>
      </div>
    </div>
  </section>

  <div v-if="showIdentityModal" class="modal-backdrop" @click.self="closeIdentityModal">
    <form class="modal" @submit.prevent="onSaveIdentity">
      <div class="modal-head">
        <div><div class="eyebrow">Kamera-Identitäten</div><h2>{{ identityForm.id ? 'Identität bearbeiten' : 'Identität hinzufügen' }}</h2></div>
        <button class="btn icon sm ghost" type="button" title="Schließen" aria-label="Schließen" @click="closeIdentityModal">×</button>
      </div>
      <div class="identity-form-grid">
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

  <div v-if="deleteCandidate" class="modal-backdrop" @click.self="closeDeleteModal">
    <div class="modal confirm-modal" role="dialog" aria-modal="true" aria-labelledby="identity-delete-title">
      <div class="modal-head">
        <div><div class="eyebrow">Entfernen</div><h2 id="identity-delete-title">Identität entfernen?</h2></div>
        <button class="btn icon sm ghost" type="button" title="Schließen" aria-label="Schließen" @click="closeDeleteModal">×</button>
      </div>
      <p class="mono-mute">„{{ deleteCandidate.name }}“ wird aus den gespeicherten Kamera-Identitäten entfernt. Bestehende Kamera-Zuordnungen bleiben unverändert.</p>
      <div class="modal-foot">
        <span></span>
        <div class="btn-row">
          <button class="btn ghost" type="button" @click="closeDeleteModal">Abbrechen</button>
          <button class="btn danger" type="button" :disabled="deletingIdentity" @click="onDeleteIdentity">{{ deletingIdentity ? 'Entfernt…' : 'Entfernen' }}</button>
        </div>
      </div>
    </div>
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
const deletingIdentity = ref(false)
const deleteCandidate = ref<CredentialIdentity | null>(null)
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
async function duplicateIdentity(identity: CredentialIdentity) {
  savingIdentity.value = true
  try {
    await saveCredentialIdentity({
      name: `${identity.name} Kopie`,
      username: identity.username,
      copy_password_from_id: identity.id
    })
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Identität konnte nicht dupliziert werden.'
  } finally {
    savingIdentity.value = false
  }
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
function askDeleteIdentity(identity: CredentialIdentity) {
  deleteCandidate.value = identity
}
function closeDeleteModal() {
  if (!deletingIdentity.value) deleteCandidate.value = null
}
async function onDeleteIdentity() {
  if (!deleteCandidate.value) return
  deletingIdentity.value = true
  try {
    await deleteCredentialIdentity(deleteCandidate.value.id)
    deleteCandidate.value = null
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Identität konnte nicht entfernt werden.'
  } finally {
    deletingIdentity.value = false
  }
}

onMounted(() => void loadAll())
</script>
