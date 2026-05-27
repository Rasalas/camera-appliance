<template>
  <header class="topline">
    <div>
      <div class="eyebrow">System · Einstellungen · Sicherung · Protokoll</div>
      <h1 class="headline">System.</h1>
    </div>
    <div class="meta">
      <div>Adresse · <b>{{ settings.bind_addr || '127.0.0.1:8091' }}</b></div>
      <div>AgentDVR · <b>{{ settings.agentdvr_url || 'http://localhost:8090' }}</b></div>
    </div>
  </header>

  <div v-if="error" class="notice err"><span class="tag">FEHLER</span>{{ error }}</div>

  <!-- Section: Settings -->
  <section class="panel">
    <div class="panel-head">
      <h2>Einstellungen</h2>
      <button class="btn sm primary" @click="saveSettings">Speichern</button>
    </div>

    <div class="split">
      <div class="field">
        <span class="lbl">Kamera-Passwort</span>
        <div class="btn-row" style="align-items: stretch;">
          <input v-model="cameraPassword" type="password" :placeholder="settings.camera_password_set === 'true' ? '••••••••••••' : 'Passwort setzen'" style="flex: 1;" />
          <button class="btn" :disabled="!cameraPassword || savingPassword" @click="saveCameraPassword">
            {{ savingPassword ? 'Speichert…' : 'Passwort speichern' }}
          </button>
        </div>
        <div class="mono-mute" style="margin-top: 6px;">
          {{ settings.camera_password_set === 'true' ? `Gespeichert über ${passwordSource}` : 'Noch kein Kamera-Passwort gespeichert.' }}
        </div>
      </div>
      <div class="field">
        <span class="lbl">AgentDVR-URL</span>
        <input v-model="settings.agentdvr_url" placeholder="http://localhost:8090" />
      </div>
      <div class="field">
        <span class="lbl">go2rtc-URL</span>
        <input v-model="settings.go2rtc_url" placeholder="http://localhost:1984" />
      </div>
      <div class="field">
        <span class="lbl">Admin-Adresse</span>
        <input v-model="settings.bind_addr" placeholder="127.0.0.1:8091" />
      </div>
      <div class="field">
        <span class="lbl">Capture-Hop per SSH</span>
        <input v-model="settings.capture_ssh_host" placeholder="leer oder nas" />
        <div class="mono-mute" style="margin-top: 6px;">
          Optional. Wenn gesetzt, zieht die App Referenzbilder per ffmpeg auf diesem SSH-Host.
        </div>
      </div>
    </div>

    <div style="display: grid; gap: 8px;">
      <label class="toggle-row">
        <input type="checkbox" :checked="settings.auto_discover === 'true'" @change="setBool('auto_discover', $event)" />
        <div>
          <div class="lbl-main">Beim Start automatisch suchen</div>
          <div class="lbl-sub">Discovery läuft direkt nach dem Boot.</div>
        </div>
      </label>
      <label class="toggle-row">
        <input type="checkbox" :checked="settings.render_after_discovery === 'true'" @change="setBool('render_after_discovery', $event)" />
        <div>
          <div class="lbl-main">go2rtc nach Suche erzeugen</div>
          <div class="lbl-sub">Neue Konfiguration wird automatisch geschrieben.</div>
        </div>
      </label>
      <label class="toggle-row">
        <input type="checkbox" :checked="settings.restart_after_render === 'true'" @change="setBool('restart_after_render', $event)" />
        <div>
          <div class="lbl-main">go2rtc nach Änderungen neu starten</div>
          <div class="lbl-sub">Streams stehen sofort am Player bereit.</div>
        </div>
      </label>
    </div>
  </section>

  <!-- Section: Credential identities -->
  <section class="panel">
    <div class="panel-head">
      <h2>Kamera-Identitäten</h2>
      <div class="device-head-actions">
        <div class="right">{{ credentialIdentities.length }} gespeichert</div>
        <button class="btn icon sm" type="button" title="Identität hinzufügen" @click="openNewIdentityModal">+</button>
      </div>
    </div>

    <div class="mono-mute">
      Identitäten sind wiederverwendbare Logins. Stream-Auswahl bleibt an Kamera, Zuordnung oder Bildtest.
    </div>

    <div v-if="!credentialIdentities.length" class="empty">Noch keine Identitäten gespeichert.</div>
    <div v-else class="result-list">
      <div v-for="identity in credentialIdentities" :key="identity.id" class="result-row ok identity-row">
        <span class="slot">Login</span>
        <span class="name">{{ identity.name }}</span>
        <span class="ip">{{ identity.username }}</span>
        <span class="stream">{{ identity.password_set ? passwordSourceLabel(identity.password_source) : 'kein Passwort' }}</span>
        <button class="btn sm ghost" type="button" @click="editCredentialIdentity(identity)">Bearbeiten</button>
        <button class="btn sm danger" type="button" @click="deleteCredentialIdentity(identity.id)">Entfernen</button>
      </div>
    </div>
  </section>

  <div v-if="showIdentityModal" class="modal-backdrop" @click.self="closeIdentityModal">
    <form class="modal" @submit.prevent="saveCredentialIdentity">
      <div class="modal-head">
        <div>
          <div class="eyebrow">Kamera-Identitäten</div>
          <h2>{{ identityForm.id ? 'Identität bearbeiten' : 'Identität hinzufügen' }}</h2>
        </div>
        <button class="btn icon sm ghost" type="button" title="Schließen" @click="closeIdentityModal">×</button>
      </div>
      <div class="split">
        <div class="field">
          <span class="lbl">Name</span>
          <input v-model="identityForm.name" placeholder="Tapo Außenkameras" autofocus />
        </div>
        <div class="field">
          <span class="lbl">Benutzername</span>
          <input v-model="identityForm.username" placeholder="Kamera-Benutzer" />
        </div>
        <div class="field">
          <span class="lbl">Passwort</span>
          <input v-model="identityForm.password" type="password" :placeholder="identityForm.id ? 'leer lassen, um Passwort zu behalten' : 'Kamera-Passwort'" />
        </div>
      </div>
      <div class="modal-foot">
        <span class="mono-mute">Wird beim Bildtest auf passende Kameras ausprobiert.</span>
        <div class="btn-row">
          <button class="btn ghost" type="button" @click="closeIdentityModal">Abbrechen</button>
          <button class="btn primary" type="submit" :disabled="savingIdentity || !identityForm.name || !identityForm.username">
            {{ savingIdentity ? 'Speichert…' : 'Speichern' }}
          </button>
        </div>
      </div>
    </form>
  </div>

  <!-- Section: Backup -->
  <section class="panel">
    <div class="panel-head">
      <h2>Sicherung</h2>
      <div class="right">Lokale Konfiguration · Bindings · Einstellungen</div>
    </div>

    <div class="split">
      <div class="field">
        <span class="lbl">Backup erstellen</span>
        <div class="btn-row">
          <button class="btn primary" @click="createBackup">Backup jetzt erstellen</button>
        </div>
      </div>
      <div class="field">
        <span class="lbl">Backup wiederherstellen</span>
        <div class="btn-row" style="align-items: stretch;">
          <input v-model="restorePath" placeholder="/var/lib/camera-appliance/backups/…" style="flex: 1;" />
          <button class="btn" :disabled="!restorePath" @click="restoreBackup">Wiederherstellen</button>
        </div>
      </div>
    </div>

    <div v-if="backupResult" class="notice ok">
      <span class="tag">FERTIG</span>
      <div>
        <div>{{ backupResult.path }}</div>
        <div v-if="backupResult.warning" class="mono-mute" style="margin-top: 4px;">{{ backupResult.warning }}</div>
      </div>
    </div>
  </section>

  <!-- Section: Events -->
  <section class="panel">
    <div class="panel-head">
      <h2>Ereignisprotokoll</h2>
      <div class="right">{{ events.length }} Einträge</div>
    </div>
    <div v-if="!events.length" class="empty">Noch keine Ereignisse vorhanden.</div>
    <div v-else class="ticker">
      <div v-for="ev in events" :key="ev.id" class="row">
        <span class="time">{{ formatTime(ev.created_at) }}</span>
        <span class="lvl" :class="levelClass(ev.level)">{{ ev.level }}</span>
        <span><b style="color: var(--ink); font-weight: 500;">{{ ev.type }}</b> · {{ ev.message }}</span>
      </div>
    </div>
  </section>

  <div class="toast-host">
    <transition name="page"><div v-if="toast" class="toast" :key="toast">{{ toast }}</div></transition>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api } from '../api/client'
import type { CredentialIdentity, EventItem } from '../types'

const settings = reactive<Record<string, string>>({})
const events = ref<EventItem[]>([])
const credentialIdentities = ref<CredentialIdentity[]>([])
const restorePath = ref('')
const backupResult = ref<{ path: string; warning: string }>()
const error = ref('')
const toast = ref('')
const cameraPassword = ref('')
const savingPassword = ref(false)
const savingIdentity = ref(false)
const showIdentityModal = ref(false)
const passwordSource = ref('unbekannt')
const identityForm = reactive({ id: '', name: '', username: '', password: '' })

function setBool(key: string, e: Event) {
  const target = e.target as HTMLInputElement
  settings[key] = target.checked ? 'true' : 'false'
}

function formatTime(t: string) {
  return new Date(t).toLocaleString('de-DE', {
    day: '2-digit', month: '2-digit',
    hour: '2-digit', minute: '2-digit'
  })
}
function levelClass(l: string) {
  const lower = (l || '').toLowerCase()
  if (lower.includes('err') || lower.includes('fail')) return 'err'
  if (lower.includes('warn')) return 'warn'
  if (lower.includes('ok') || lower.includes('info')) return 'ok'
  return ''
}
function passwordSourceLabel(source?: string) {
  if (!source) return 'Passwort gespeichert'
  if (source === 'keyring') return 'Keyring'
  if (source === 'local.env') return 'Secret-Datei'
  return source
}

async function saveSettings() {
  try {
    await api.saveSettings(settings)
    toast.value = 'Einstellungen gespeichert'
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Speichern fehlgeschlagen.'
  }
}

async function saveCameraPassword() {
  savingPassword.value = true
  error.value = ''
  try {
    const result = await api.saveCameraPassword(cameraPassword.value)
    settings.camera_password_set = 'true'
    settings.camera_password_source = result.source
    passwordSource.value = result.source === 'keyring' ? 'Betriebssystem-Keyring' : result.source
    cameraPassword.value = ''
    toast.value = 'Kamera-Passwort gespeichert'
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Passwort konnte nicht gespeichert werden.'
  } finally {
    savingPassword.value = false
  }
}

function resetIdentityForm() {
  identityForm.id = ''
  identityForm.name = ''
  identityForm.username = ''
  identityForm.password = ''
}

function openNewIdentityModal() {
  resetIdentityForm()
  showIdentityModal.value = true
}

function closeIdentityModal() {
  if (!savingIdentity.value) showIdentityModal.value = false
}

function editCredentialIdentity(identity: CredentialIdentity) {
  identityForm.id = identity.id
  identityForm.name = identity.name
  identityForm.username = identity.username
  identityForm.password = ''
  showIdentityModal.value = true
}

async function loadCredentialIdentities() {
  credentialIdentities.value = await api.credentialIdentities()
}

async function saveCredentialIdentity() {
  savingIdentity.value = true
  error.value = ''
  try {
    await api.saveCredentialIdentity({
      id: identityForm.id || undefined,
      name: identityForm.name,
      username: identityForm.username,
      password: identityForm.password || undefined
    })
    await loadCredentialIdentities()
    resetIdentityForm()
    showIdentityModal.value = false
    toast.value = 'Identität gespeichert'
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Identität konnte nicht gespeichert werden.'
  } finally {
    savingIdentity.value = false
  }
}

async function deleteCredentialIdentity(id: string) {
  try {
    await api.deleteCredentialIdentity(id)
    await loadCredentialIdentities()
    if (identityForm.id === id) resetIdentityForm()
    toast.value = 'Identität entfernt'
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Identität konnte nicht entfernt werden.'
  }
}

async function createBackup() {
  try {
    backupResult.value = await api.backup()
    toast.value = 'Backup erstellt'
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Backup konnte nicht erstellt werden.'
  }
}

async function restoreBackup() {
  try {
    backupResult.value = await api.restore(restorePath.value)
    toast.value = 'Backup wiederhergestellt'
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Wiederherstellung fehlgeschlagen.'
  }
}

onMounted(async () => {
  try {
    Object.assign(settings, await api.settings())
    passwordSource.value = settings.camera_password_source === 'keyring' ? 'Betriebssystem-Keyring' : (settings.camera_password_source || 'unbekannt')
    await loadCredentialIdentities()
    events.value = await api.events()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Konnte nicht geladen werden.'
  }
})
</script>
