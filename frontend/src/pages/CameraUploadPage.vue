<template>
  <header class="topline"><div><RouterLink v-if="editing" class="mono-mute" to="/kameras/bild-upload">← Bild-Upload</RouterLink><h1 class="headline">{{ editing ? 'Upload-Server bearbeiten' : 'Bild-Upload' }}</h1></div><div v-if="editing" class="editor-actions"><button class="btn primary" type="submit" form="upload-server" :disabled="busy || !dirty">Server speichern</button><RouterLink class="btn" to="/kameras/bild-upload">Abbrechen</RouterLink></div></header>
  <EditableSection v-if="!editing" title="Server für Einzelbilder" to="/kameras/bild-upload/bearbeiten" :disabled="loading">
    <p class="mono-mute">Gemeinsamer Server für Kamera-Uploads. Bildausschnitt und Zeitplan stellst du bei der jeweiligen Kamera ein.</p>
    <p v-if="loading" role="status">Einstellungen werden geladen…</p>
    <p v-if="error" class="notice err" role="alert">{{ error }}</p>
    <dl v-if="!loading && baseline" class="spec"><div><dt>Protokoll</dt><dd>{{ form.protocol.toUpperCase() }}</dd></div><div><dt>Server</dt><dd>{{ form.host || 'Noch nicht eingerichtet' }}</dd></div><div><dt>Benutzername</dt><dd>{{ form.username || 'Nicht gesetzt' }}</dd></div><div><dt>Verzeichnis</dt><dd>{{ form.directory }}</dd></div><div><dt>Passwort</dt><dd>{{ passwordSet ? 'Gespeichert' : 'Nicht gesetzt' }}</dd></div></dl>
  </EditableSection>
  <section v-else class="panel card">
    <div class="panel-head"><h2>Server für Einzelbilder</h2></div>
    <p class="mono-mute">Gemeinsamer Server für die Kamera-Uploads. Bildausschnitt, Privatbereiche, Zeitangabe und Zeitplan stellst du bei der jeweiligen Kamera ein.</p>
    <div v-if="error" class="notice err" role="alert">{{ error }}</div>
    <div v-if="message && !dirty" class="notice ok" role="status">{{ message }}</div>
    <div v-if="loading" class="empty">Einstellungen werden geladen…</div>

    <form v-if="!loading" id="upload-server" @submit.prevent="save">
      <fieldset :disabled="busy || !baseline" class="upload-fields">
        <div class="split">
          <label class="field"><span class="lbl">Protokoll</span><select aria-label="Protokoll" v-model="form.protocol" @change="changeProtocol"><option value="sftp">SFTP · verschlüsselt</option><option value="ftp">FTP · unverschlüsselt</option></select></label>
          <label class="field"><span class="lbl">Server</span><input aria-label="Server" v-model="form.host" required placeholder="bilder.example.org" autocomplete="off" /></label>
          <label class="field"><span class="lbl">Port</span><input aria-label="Port" v-model.number="form.port" type="number" min="1" max="65535" required /></label>
          <label class="field"><span class="lbl">Benutzername</span><input aria-label="Benutzername" v-model="form.username" required autocomplete="off" /></label>
          <label class="field"><span class="lbl">Serverpasswort</span><input aria-label="Serverpasswort" v-model="password" type="password" autocomplete="new-password" :disabled="clearPassword" :placeholder="passwordSet ? 'Leer lassen, um gespeichertes Passwort zu behalten' : 'Passwort eingeben'" /></label>
          <label class="field"><span class="lbl">Standardverzeichnis</span><input aria-label="Standardverzeichnis" v-model="form.directory" placeholder=". oder /bilder" /><span class="mono-mute">Gilt für Kameras ohne eigenes Verzeichnis. Der Ordner muss vorhanden und beschreibbar sein. „.“ verwendet das Anmeldeverzeichnis.</span></label>
        </div>
        <label v-if="form.protocol === 'sftp'" class="field"><span class="lbl">SSH-Hostschlüssel · SHA256-Fingerabdruck</span><input aria-label="SSH-Hostschlüssel · SHA256-Fingerabdruck" v-model="form.host_key" required placeholder="SHA256:…" autocomplete="off" /><span class="mono-mute">Den Fingerabdruck erhältst du vom Serverbetreiber. Er bestätigt, dass die Verbindung zum richtigen Server aufgebaut wird.</span></label>
        <div v-else class="notice">FTP überträgt Bilder und Zugangsdaten unverschlüsselt. Verwende SFTP, wenn dein Server es unterstützt.</div>
        <label v-if="passwordSet" class="toggle-row"><input v-model="clearPassword" type="checkbox" @change="password = ''" /><span>Gespeichertes Upload-Passwort löschen</span></label>
        <p class="mono-mute">Ein leeres Passwortfeld behält das Passwort für denselben Server und Benutzer. Bei geändertem Server, Protokoll, Port, Benutzer oder SSH-Hostschlüssel ist ein neues Passwort nötig.</p>
        <div class="form-actions">
          <span class="mono-mute" role="status">{{ error ? 'Nicht gespeichert' : dirty ? 'Ungespeicherte Änderungen' : 'Gespeichert' }}</span>

        </div>
      </fieldset>
    </form>
  </section>
  <RouterLink v-if="!editing" class="mobile-fab" aria-label="Upload-Server bearbeiten" to="/kameras/bild-upload/bearbeiten"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m4 16 12-12 4 4-12 12H4zM14 6l4 4"/></svg></RouterLink>
</template>

<script setup lang="ts">
import EditableSection from '../components/EditableSection.vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDraftGuard } from '../composables/discardChanges'
import { api } from '../api/client'
import type { UploadSettings } from '../types'

const route=useRoute(),router=useRouter()
const editing=computed(()=>route.path.endsWith('/bearbeiten'))
useDraftGuard(()=>editing.value && !loading.value && dirty.value,cancel)
const form = reactive<Omit<UploadSettings, 'password_set'>>({ protocol: 'sftp', host: '', port: 22, username: '', directory: '.', host_key: '' })
const password = ref('')
const passwordSet = ref(false)
const clearPassword = ref(false)
const busy = ref(false)
const loading = ref(true)
const error = ref('')
const message = ref('')
const baseline = ref('')
const dirty = computed(() => JSON.stringify(form) !== baseline.value || !!password.value || clearPassword.value)

function apply(settings: UploadSettings) {
  const { password_set, ...config } = settings
  Object.assign(form, config)
  baseline.value = JSON.stringify(form)
  passwordSet.value = password_set
}
function cancel() {
  Object.assign(form, JSON.parse(baseline.value))
  password.value = ''
  clearPassword.value = false
  error.value = ''
  message.value = ''
}
function changeProtocol() {
  form.port = form.protocol === 'sftp' ? 22 : 21
  if (form.protocol === 'ftp') form.host_key = ''
}
async function save() {
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    apply(await api.saveUploadSettings({ ...form, password: password.value, clear_password: clearPassword.value }))
    password.value = ''
    clearPassword.value = false
    await router.push('/kameras/bild-upload')
    message.value = passwordSet.value ? 'Server gespeichert. Ein Einzelbild kannst du jetzt in der Kameradetailseite hochladen.' : 'Upload-Passwort gelöscht. Für weitere Uploads ist ein neues Passwort nötig.'
  } catch (err) { error.value = err instanceof Error ? err.message : 'Server konnte nicht gespeichert werden.' }
  finally { busy.value = false }
}
onMounted(async () => {
  try { apply(await api.uploadSettings()) }
  catch (err) { error.value = err instanceof Error ? err.message : 'Einstellungen konnten nicht geladen werden.' }
  finally { loading.value = false }
})
</script>

<style scoped>
.upload-fields { border: 0; padding: 0; margin: 0; display: grid; gap: 18px; min-width: 0; }
.upload-fields .field { align-content:start; }
.form-actions { display:flex;align-items:center;justify-content:space-between;gap:14px;flex-wrap:wrap;border-top:1px solid var(--hairline);padding-top:16px; }
</style>
