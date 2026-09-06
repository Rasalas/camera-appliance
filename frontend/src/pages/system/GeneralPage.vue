<template>
  <EditableSection title="Kamera-Passwort" @edit="passwordOpen = true"><p>{{ settings.camera_password_set === 'true' ? 'Gemeinsames Kamera-Passwort gespeichert.' : 'Noch kein gemeinsames Passwort gespeichert.' }}</p></EditableSection>
  <AdminDialog ref="passwordDialog" :open="passwordOpen" title="Kamera-Passwort bearbeiten" :dirty="!!cameraPassword" :busy="savingPassword" @close="closePassword">
  <form class="password-form" @submit.prevent="saveCamPassword">
    <p class="mono-mute">Gemeinsames Passwort für Kameras ohne eigenen gespeicherten Zugang.</p>
    <label class="field"><span class="lbl">Kamera-Passwort</span><input aria-label="Kamera-Passwort" v-model="cameraPassword" type="password" :disabled="savingPassword" autocomplete="new-password" :placeholder="settings.camera_password_set === 'true' ? '••••••••••••' : 'Passwort setzen'" /></label>
    <p v-if="settings.camera_password_set === 'true'" class="mono-mute">Gespeichert über {{ passwordSource }}</p>
    <div v-if="passwordError" role="alert" class="notice err">{{ passwordError }}</div>
    <div class="form-actions"><button class="btn ghost" type="button" :disabled="savingPassword" @click="passwordDialog?.requestClose()">Abbrechen</button><button class="btn primary" :disabled="!cameraPassword || savingPassword" type="submit">{{ savingPassword ? 'Speichert…' : 'Passwort speichern' }}</button></div>
  </form>
  </AdminDialog>
  <SettingsForm title="Verbindungen und Automatik" :setting-keys="connectionKeys">
    <template #summary><dl class="spec"><div><dt>Streamdienst</dt><dd>{{ settings.go2rtc_url }}</dd></div><div><dt>SSH-Hop</dt><dd>{{ settings.capture_ssh_host || 'Direkt auf dieser Station' }}</dd></div><div><dt>Automatische Suche</dt><dd>{{ settings.auto_discover === 'true' ? 'Aktiv' : 'Aus' }}</dd></div></dl></template>
    <div class="split">
      <div class="field">
        <span class="lbl">go2rtc-URL</span>
        <input aria-label="go2rtc-URL" :value="settings.go2rtc_url" readonly placeholder="http://localhost:1984" />
      </div>
      <div class="field">
        <span class="lbl">Capture-Hop per SSH</span>
        <input aria-label="Capture-Hop per SSH" v-model="settings.capture_ssh_host" placeholder="leer oder nas" />
        <div class="mono-mute" style="margin-top: 6px;">Optional. Wenn gesetzt, zieht die App Referenzbilder per ffmpeg auf diesem SSH-Host.</div>
      </div>
    </div>

    <div style="display: grid; gap: 8px;">
      <label class="toggle-row">
        <input type="checkbox" :checked="settings.auto_discover === 'true'" @change="setBool('auto_discover', $event)" />
        <div><div class="lbl-main">Beim Start automatisch suchen</div><div class="lbl-sub">Discovery läuft direkt nach dem Boot.</div></div>
      </label>
      <label class="toggle-row">
        <input type="checkbox" :checked="settings.render_after_discovery === 'true'" @change="setBool('render_after_discovery', $event)" />
        <div><div class="lbl-main">go2rtc nach Suche erzeugen</div><div class="lbl-sub">Neue Konfiguration wird automatisch geschrieben.</div></div>
      </label>
      <label class="toggle-row">
        <input type="checkbox" :checked="settings.restart_after_render === 'true'" @change="setBool('restart_after_render', $event)" />
        <div><div class="lbl-main">go2rtc nach Änderungen neu starten</div><div class="lbl-sub">Streams stehen sofort am Player bereit.</div></div>
      </label>
    </div>
  </SettingsForm>

  <SettingsForm title="Anzeige" :setting-keys="['viewer.performance.mode']">
    <template #summary><p>{{ viewerPerformanceOptions.find(option => option.id === settings['viewer.performance.mode'])?.name || 'Qualität' }}</p><p class="mono-mute">{{ viewerPerformanceDescription }}</p></template>
    <div class="split">
      <div class="field">
        <span class="lbl">Performance</span>
        <select aria-label="Performance" v-model="settings['viewer.performance.mode']">
          <option v-for="option in viewerPerformanceOptions" :key="option.id" :value="option.id">{{ option.name }}</option>
        </select>
        <div class="mono-mute" style="margin-top: 6px;">{{ viewerPerformanceDescription }}</div>
      </div>
      <div class="field">
        <span class="lbl">Kiosk</span>
        <div class="btn-row"><RouterLink class="btn" to="/">Live-Ansicht öffnen</RouterLink></div>
      </div>
    </div>
  </SettingsForm>
</template>

<script setup lang="ts">
import EditableSection from '../../components/EditableSection.vue'
import { onMounted, ref } from 'vue'
import { useSystem } from '../../composables/useSystem'
import { generalSettingKeys } from '../../composables/settingsDraft'
import AdminDialog from '../../components/AdminDialog.vue'
import { useDraftGuard } from '../../composables/discardChanges'
import SettingsForm from '../../components/SettingsForm.vue'
const connectionKeys = generalSettingKeys.filter(key => key !== 'viewer.performance.mode')

const { settings, passwordSource, viewerPerformanceOptions, viewerPerformanceDescription, loadAll, saveCameraPassword, setBool } = useSystem()
const cameraPassword = ref('')
const passwordOpen = ref(false), passwordDialog = ref<InstanceType<typeof AdminDialog>>()
function closePassword() { cameraPassword.value = ''; passwordError.value = ''; passwordOpen.value = false }
useDraftGuard(() => passwordOpen.value && !!cameraPassword.value, closePassword)
const savingPassword = ref(false)
const passwordMessage=ref(''),passwordError=ref('')

async function saveCamPassword() {
  if(savingPassword.value)return
  savingPassword.value = true
  passwordMessage.value='';passwordError.value=''
  try {
    await saveCameraPassword(cameraPassword.value)
    cameraPassword.value = ''
    passwordMessage.value='Kamera-Passwort gespeichert.'
    passwordOpen.value = false
  } catch (err) {
    passwordError.value = err instanceof Error ? err.message : 'Passwort konnte nicht gespeichert werden.'
  } finally {
    savingPassword.value = false
  }
}

onMounted(() => void loadAll())
</script>
