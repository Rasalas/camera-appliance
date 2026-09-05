<template>
  <form class="panel card" @submit.prevent="saveCamPassword">
    <div class="panel-head"><h2>Kamera-Passwort</h2></div>
    <p class="mono-mute">Gemeinsames Passwort für Kameras ohne eigenen gespeicherten Zugang.</p>
      <div class="field">
        <span class="lbl">Kamera-Passwort</span>
        <div class="btn-row" style="align-items: stretch;">
          <input v-model="cameraPassword" type="password" :disabled="savingPassword" :placeholder="settings.camera_password_set === 'true' ? '••••••••••••' : 'Passwort setzen'" style="flex: 1;" />
          <button class="btn ghost" type="button" :disabled="!cameraPassword || savingPassword" @click="cameraPassword = ''; passwordError = ''; passwordMessage = ''">Abbrechen</button>
          <button class="btn" :disabled="!cameraPassword || savingPassword" type="submit">
            {{ savingPassword ? 'Speichert…' : 'Passwort speichern' }}
          </button>
        </div>
        <div class="mono-mute" style="margin-top: 6px;">
          {{ settings.camera_password_set === 'true' ? `Gespeichert über ${passwordSource}` : 'Noch kein Kamera-Passwort gespeichert.' }}
        </div>
      </div>
    <div v-if="cameraPassword" role="status" class="mono-mute">Ungespeichertes Passwort</div>
    <div v-else-if="passwordMessage" role="status" class="mono-mute">{{ passwordMessage }}</div>
    <div v-if="passwordError" role="alert" class="notice err">{{ passwordError }}</div>
  </form>
  <SettingsForm title="Verbindungen und Automatik" :setting-keys="connectionKeys">
    <div class="split">
      <div class="field">
        <span class="lbl">go2rtc-URL</span>
        <input :value="settings.go2rtc_url" readonly placeholder="http://localhost:1984" />
      </div>
      <div class="field">
        <span class="lbl">Capture-Hop per SSH</span>
        <input v-model="settings.capture_ssh_host" placeholder="leer oder nas" />
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
    <div class="split">
      <div class="field">
        <span class="lbl">Performance</span>
        <select v-model="settings['viewer.performance.mode']">
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
import { onMounted, ref } from 'vue'
import { useSystem } from '../../composables/useSystem'
import { generalSettingKeys } from '../../composables/settingsDraft'
import SettingsForm from '../../components/SettingsForm.vue'
const connectionKeys = generalSettingKeys.filter(key => key !== 'viewer.performance.mode')

const { settings, passwordSource, viewerPerformanceOptions, viewerPerformanceDescription, loadAll, saveCameraPassword, setBool } = useSystem()
const cameraPassword = ref('')
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
  } catch (err) {
    passwordError.value = err instanceof Error ? err.message : 'Passwort konnte nicht gespeichert werden.'
  } finally {
    savingPassword.value = false
  }
}

onMounted(() => void loadAll())
</script>
