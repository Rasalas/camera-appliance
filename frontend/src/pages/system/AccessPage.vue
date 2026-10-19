<template>
  <p class="mono-mute">Lege fest, wo die Station erreichbar ist und wer Einstellungen oder Kamerabilder öffnen darf.</p>
  <div v-if="restart === 'running'" class="notice restart-notice" role="status" aria-live="polite">
    <span class="tag">NEUSTART</span>
    <div class="restart-body">
      <div>Station startet neu… {{ restartSeconds }} s</div>
      <div class="mono-mute">Die Seite lädt automatisch neu, sobald die Station wieder antwortet. Das kann bis zu drei Minuten dauern.</div>
      <div class="progress" />
    </div>
  </div>
  <div v-else-if="restart === 'timeout'" class="notice err" role="alert">
    <span class="tag">FEHLER</span>
    <div class="restart-body">
      <div>Die Station antwortet nach drei Minuten noch nicht.</div>
      <div>Prüfe, ob das Gerät eingeschaltet und mit dem Netzwerk verbunden ist, und lade die Seite dann neu. Hilft das nicht, schalte das Gerät aus und wieder ein.</div>
      <div><button class="btn" type="button" @click="reloadPage">Seite neu laden</button></div>
    </div>
  </div>
  <div v-else-if="restart === 'failed'" class="notice err" role="alert"><span class="tag">FEHLER</span>Neustart fehlgeschlagen ({{ restartError }}). Die Einstellung ist gespeichert und wird beim nächsten Neustart der Station wirksam.</div>
  <SettingsForm title="Netzwerk" :setting-keys="['network.lan_access_enabled']" :after-save="startNetworkRestart">
    <template #summary><p>{{ settings['network.lan_access_enabled'] === 'true' ? 'Die Station ist im lokalen Netzwerk erreichbar.' : 'Zugriff ist auf dieses Gerät beschränkt.' }}</p><p class="mono-mute">Eine Änderung startet den Dienst neu.</p></template>
    <label class="toggle-row"><input type="checkbox" :checked="settings['network.lan_access_enabled'] === 'true'" :disabled="settings.auth_admin_password_set !== 'true'" @change="setBool('network.lan_access_enabled',$event)" /><span>Zugriff aus dem lokalen Netzwerk erlauben</span></label>
    <p v-if="settings.auth_admin_password_set !== 'true'" class="notice warn">Setze zuerst unter Administration ein Admin-Passwort, um den Netzwerkzugriff freizugeben.</p>
    <p class="mono-mute">Speichern wendet den Netzwerkzugriff an und startet die Station neu.</p>
  </SettingsForm>
  <SettingsForm title="Administration" :setting-keys="['auth.session_hours', 'auth.local_admin_bypass']">
    <template #summary><dl class="spec"><div><dt>Admin-Passwort</dt><dd>{{ passwordStatus('admin') }}</dd></div><div><dt>Lokaler Zugriff</dt><dd>{{ settings['auth.local_admin_bypass'] === 'true' ? 'Ohne Anmeldung am Host' : 'Anmeldung erforderlich' }}</dd></div><div><dt>Sitzungsdauer</dt><dd>{{ settings['auth.session_hours'] }} Stunden für Admin und Viewer</dd></div></dl><button class="btn ghost" @click="openPassword('admin')">Admin-Passwort ändern</button></template>
    <label class="toggle-row"><input type="checkbox" :checked="settings['auth.local_admin_bypass'] === 'true'" @change="setBool('auth.local_admin_bypass',$event)" /><span>Lokalen Host als Admin akzeptieren</span></label>
    <label class="field"><span class="lbl">Sitzungsdauer · Stunden</span><input aria-label="Sitzungsdauer · Stunden" v-model="settings['auth.session_hours']" type="number" min="1" max="168" required /><span class="mono-mute">Gilt für Admin und Viewer. „Angemeldet bleiben“ merkt dieses Gerät 30 Tage lang.</span></label>
  </SettingsForm>
  <SettingsForm title="Live-Ansicht" :setting-keys="['auth.viewer_public']">
    <template #summary><dl class="spec"><div><dt>Kamerabilder</dt><dd>{{ settings['auth.viewer_public'] === 'true' ? 'Ohne Anmeldung erreichbar' : 'Anmeldung erforderlich' }}</dd></div><div><dt>Viewer-Passwort</dt><dd>{{ passwordStatus('viewer') }}</dd></div></dl><button class="btn ghost" @click="openPassword('viewer')">Viewer-Passwort ändern</button></template>
    <p class="mono-mute">Der Viewer sieht Kamerabilder und kann keine Einstellungen ändern.</p>
    <label class="toggle-row"><input type="checkbox" :checked="settings['auth.viewer_public'] === 'true'" @change="setBool('auth.viewer_public',$event)" /><span>Live-Ansicht ohne Anmeldung erlauben</span></label>
    <p v-if="settings['auth.viewer_public'] !== 'true' && settings.auth_viewer_password_set !== 'true'" class="notice">Ohne Viewer-Passwort kann nur ein angemeldeter Admin die Live-Ansicht öffnen.</p>
  </SettingsForm>
  <AdminDialog ref="dialog" :open="!!role" :title="role === 'admin' ? 'Admin-Passwort bearbeiten' : 'Viewer-Passwort bearbeiten'" :dirty="!!password" :busy="saving" @close="closePassword">
    <form @submit.prevent="savePassword"><label class="field"><span class="lbl">Neues Passwort · Pflichtfeld</span><input aria-label="Neues Passwort · Pflichtfeld" v-model="password" type="password" :disabled="saving" required autocomplete="new-password" autofocus /></label><p v-if="passwordError" class="notice err" role="alert">{{ passwordError }}</p><div class="form-actions"><button class="btn" type="button" :disabled="saving" @click="dialog?.requestClose()">Abbrechen</button><button class="btn primary" :disabled="saving" type="submit">Passwort speichern</button></div></form>
  </AdminDialog>
</template>
<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '../../api/client'
import { useSystem } from '../../composables/useSystem'
import { useDraftGuard } from '../../composables/discardChanges'
import { restartStation } from '../../composables/stationRestart'
import SettingsForm from '../../components/SettingsForm.vue'
import AdminDialog from '../../components/AdminDialog.vue'
import type { AuthRole } from '../../types'
const { settings, setBool, loadAll, saveAuthPassword } = useSystem()
const role = ref<AuthRole | ''>(''), password = ref(''), saving = ref(false), passwordError = ref('')
const dialog = ref<InstanceType<typeof AdminDialog>>()
function passwordStatus(value: AuthRole) { return settings[`auth_${value}_password_set`] === 'true' ? 'Gespeichert' : 'Noch nicht gesetzt' }
function openPassword(value: AuthRole) { role.value = value; password.value = ''; passwordError.value = '' }
function closePassword() { role.value = ''; password.value = ''; passwordError.value = '' }
useDraftGuard(() => !!role.value && !!password.value, closePassword)
async function savePassword() {
  if (!role.value || saving.value) return
  saving.value = true; passwordError.value = ''
  try { await saveAuthPassword(role.value, password.value); closePassword() }
  catch (err) { passwordError.value = err instanceof Error ? err.message : 'Passwort konnte nicht gespeichert werden.' }
  finally { saving.value = false }
}
const restart = ref<'idle' | 'running' | 'timeout' | 'failed'>('idle'), restartSeconds = ref(0), restartError = ref('')
let disposed = false
function reloadPage() { window.location.reload() }
async function applyNetwork() {
  if (restart.value === 'running') return
  restart.value = 'running'; restartSeconds.value = 0; restartError.value = ''
  const outcome = await restartStation({
    trigger: () => api.restartStack(),
    probe: () => api.health({ timeoutMs: 3000 }).then(() => true, () => false),
    sleep: ms => new Promise(resolve => window.setTimeout(resolve, ms)),
    now: () => Date.now(),
    onProgress: seconds => { restartSeconds.value = seconds }
  })
  if (disposed) return
  if (outcome.kind === 'ready') return reloadPage()
  if (outcome.kind === 'failed') restartError.value = outcome.message
  restart.value = outcome.kind
}
// Returns at once so the dialog closes; progress is shown on the page.
async function startNetworkRestart() { void applyNetwork() }
onMounted(() => void loadAll())
onBeforeUnmount(() => { disposed = true })
</script>
<style scoped>
.restart-body { display: grid; gap: 8px; flex: 1; }
.restart-notice .progress { margin-top: 4px; }
</style>
