<template>
  <section class="panel card">
    <div class="panel-head">
      <h2>Zugriff aus dem Netzwerk</h2>
      <button class="btn sm primary" :disabled="applyingNetwork" @click="applyAccessSettings">{{ applyingNetwork ? 'Server startet neu…' : 'Speichern und anwenden' }}</button>
    </div>
    <label class="toggle-row">
      <input type="checkbox" :checked="settings['network.lan_access_enabled'] === 'true'" :disabled="settings.auth_admin_password_set !== 'true'" @change="setBool('network.lan_access_enabled', $event)" />
      <div><div class="lbl-main">Zugriff aus dem lokalen Netzwerk erlauben</div><div class="lbl-sub">Die Kameraansicht ist danach über die lokale IP auf Port 8091 erreichbar.</div></div>
    </label>
    <div v-if="settings.auth_admin_password_set !== 'true'" class="notice warn"><span class="tag">SCHUTZ</span>Setze zuerst ein Admin-Passwort, bevor du die Anwendung im Netzwerk freigibst.</div>
    <div v-else-if="settings['network.lan_access_enabled'] === 'true'" class="notice ok"><span class="tag">LAN</span>Netzwerkzugriff ist aktiviert. Falls das Gerät nicht erreichbar ist, muss TCP-Port 8091 in UFW freigegeben werden.</div>
  </section>

  <section class="panel card">
    <div class="panel-head"><h2>Login</h2></div>
    <div class="split">
      <div class="field">
        <span class="lbl">Admin-Login</span>
        <div class="btn-row" style="align-items: stretch;"><input v-model="adminPassword" type="password" :placeholder="settings.auth_admin_password_set === 'true' ? 'Neues Admin-Passwort' : 'Admin-Passwort setzen'" style="flex: 1;" /><button class="btn" :disabled="!adminPassword || saving === 'admin'" @click="saveAuth('admin')">{{ saving === 'admin' ? 'Speichert…' : 'Speichern' }}</button></div>
        <div class="mono-mute" style="margin-top: 6px;">{{ settings.auth_admin_password_set === 'true' ? 'Admin-Passwort ist gesetzt.' : 'Noch kein Admin-Passwort gesetzt.' }}</div>
      </div>
      <div class="field">
        <span class="lbl">Viewer-Login</span>
        <div class="btn-row" style="align-items: stretch;"><input v-model="viewerPassword" type="password" :placeholder="settings.auth_viewer_password_set === 'true' ? 'Neues Viewer-Passwort' : 'Viewer-Passwort setzen'" style="flex: 1;" /><button class="btn" :disabled="!viewerPassword || saving === 'viewer'" @click="saveAuth('viewer')">{{ saving === 'viewer' ? 'Speichert…' : 'Speichern' }}</button></div>
        <div class="mono-mute" style="margin-top: 6px;">{{ settings.auth_viewer_password_set === 'true' ? 'Viewer-Passwort ist gesetzt.' : 'Viewer-Login ist noch nicht eingerichtet.' }}</div>
      </div>
      <div class="field">
        <span class="lbl">Normale Session · Stunden</span>
        <input v-model="settings['auth.session_hours']" type="number" min="1" max="168" />
        <div class="mono-mute" style="margin-top: 6px;">„Angemeldet bleiben“ merkt dieses Gerät unabhängig davon 30 Tage lang.</div>
      </div>
    </div>
    <div style="display: grid; gap: 8px;">
      <label class="toggle-row"><input type="checkbox" :checked="settings['auth.viewer_public'] === 'true'" @change="setBool('auth.viewer_public', $event)" /><div><div class="lbl-main">Viewer ohne Login erlauben</div><div class="lbl-sub">Nur die Kameraansicht bleibt ohne Anmeldung erreichbar; Admin-Funktionen bleiben geschützt.</div></div></label>
      <label class="toggle-row"><input type="checkbox" :checked="settings['auth.local_admin_bypass'] === 'true'" @change="setBool('auth.local_admin_bypass', $event)" /><div><div class="lbl-main">Lokalen Host als Admin akzeptieren</div><div class="lbl-sub">Zugriffe direkt von 127.0.0.1 dürfen ohne Passwort konfigurieren.</div></div></label>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../../api/client'
import { useSystem } from '../../composables/useSystem'
import { accessSettingKeys } from '../../composables/settingsDraft'
import type { AuthRole } from '../../types'

const { settings, loadAll, saveAuthPassword, saveSettings, setBool, error } = useSystem()
const adminPassword = ref('')
const viewerPassword = ref('')
const saving = ref<AuthRole | ''>('')
const applyingNetwork = ref(false)

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

async function applyAccessSettings() {
  applyingNetwork.value = true
  error.value = ''
  try {
    if (!await saveSettings(accessSettingKeys)) { applyingNetwork.value = false; return }
    void api.restartStack().catch(() => undefined)
    window.setTimeout(() => window.location.reload(), 10000)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Zugriffseinstellungen konnten nicht angewendet werden.'
    applyingNetwork.value = false
  }
}

onMounted(() => void loadAll())
</script>
