<template>
  <section class="panel card">
    <div class="panel-head">
      <h2>Zugriff</h2>
      <button class="btn sm primary" @click="saveSettings()">Speichern</button>
    </div>

    <div class="split">
      <div class="field">
        <span class="lbl">Admin-Login</span>
        <div class="btn-row" style="align-items: stretch;">
          <input v-model="adminPassword" type="password" :placeholder="settings.auth_admin_password_set === 'true' ? 'Neues Admin-Passwort' : 'Admin-Passwort setzen'" style="flex: 1;" />
          <button class="btn" :disabled="!adminPassword || saving === 'admin'" @click="saveAuth('admin')">
            {{ saving === 'admin' ? 'Speichert…' : 'Speichern' }}
          </button>
        </div>
        <div class="mono-mute" style="margin-top: 6px;">
          {{ settings.auth_admin_password_set === 'true' ? 'Admin-Passwort ist gesetzt.' : 'Noch kein Admin-Passwort gesetzt.' }}
        </div>
      </div>

      <div class="field">
        <span class="lbl">Viewer-Login</span>
        <div class="btn-row" style="align-items: stretch;">
          <input v-model="viewerPassword" type="password" :placeholder="settings.auth_viewer_password_set === 'true' ? 'Neues Viewer-Passwort' : 'Viewer-Passwort setzen'" style="flex: 1;" />
          <button class="btn" :disabled="!viewerPassword || saving === 'viewer'" @click="saveAuth('viewer')">
            {{ saving === 'viewer' ? 'Speichert…' : 'Speichern' }}
          </button>
        </div>
        <div class="mono-mute" style="margin-top: 6px;">
          {{ settings.auth_viewer_password_set === 'true' ? 'Viewer-Passwort ist gesetzt.' : 'Viewer-Login ist noch nicht eingerichtet.' }}
        </div>
      </div>

      <div class="field">
        <span class="lbl">Session-Dauer · Stunden</span>
        <input v-model="settings['auth.session_hours']" type="number" min="1" max="168" />
      </div>
    </div>

    <div style="display: grid; gap: 8px;">
      <label class="toggle-row">
        <input type="checkbox" :checked="settings['auth.viewer_public'] === 'true'" @change="setBool('auth.viewer_public', $event)" />
        <div><div class="lbl-main">Viewer ohne Login erlauben</div><div class="lbl-sub">Nur die Kameraansicht bleibt ohne Anmeldung erreichbar; Admin-Funktionen bleiben geschützt.</div></div>
      </label>
      <label class="toggle-row">
        <input type="checkbox" :checked="settings['auth.local_admin_bypass'] === 'true'" @change="setBool('auth.local_admin_bypass', $event)" />
        <div><div class="lbl-main">Lokalen Host als Admin akzeptieren</div><div class="lbl-sub">Zugriffe direkt von 127.0.0.1 dürfen ohne Passwort konfigurieren.</div></div>
      </label>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useSystem } from '../../composables/useSystem'
import type { AuthRole } from '../../types'

const { settings, loadAll, saveSettings, saveAuthPassword, setBool, error } = useSystem()
const adminPassword = ref('')
const viewerPassword = ref('')
const saving = ref<AuthRole | ''>('')

async function saveAuth(role: AuthRole) {
  saving.value = role
  try {
    await saveAuthPassword(role, role === 'admin' ? adminPassword.value : viewerPassword.value)
    if (role === 'admin') adminPassword.value = ''
    else viewerPassword.value = ''
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Login-Passwort konnte nicht gespeichert werden.'
  } finally {
    saving.value = ''
  }
}

onMounted(() => void loadAll())
</script>
