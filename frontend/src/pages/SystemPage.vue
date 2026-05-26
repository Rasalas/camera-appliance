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
import type { EventItem } from '../types'

const settings = reactive<Record<string, string>>({})
const events = ref<EventItem[]>([])
const restorePath = ref('')
const backupResult = ref<{ path: string; warning: string }>()
const error = ref('')
const toast = ref('')
const cameraPassword = ref('')
const savingPassword = ref(false)
const passwordSource = ref('unbekannt')

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
    events.value = await api.events()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Konnte nicht geladen werden.'
  }
})
</script>
