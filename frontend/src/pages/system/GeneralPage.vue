<template>
  <section class="panel card">
    <div class="panel-head">
      <h2>Einstellungen</h2>
      <button class="btn sm primary" @click="saveSettings(generalSettingKeys)">Speichern</button>
    </div>

    <div class="split">
      <div class="field">
        <span class="lbl">Kamera-Passwort</span>
        <div class="btn-row" style="align-items: stretch;">
          <input v-model="cameraPassword" type="password" :placeholder="settings.camera_password_set === 'true' ? '••••••••••••' : 'Passwort setzen'" style="flex: 1;" />
          <button class="btn" :disabled="!cameraPassword || savingPassword" @click="saveCamPassword">
            {{ savingPassword ? 'Speichert…' : 'Passwort speichern' }}
          </button>
        </div>
        <div class="mono-mute" style="margin-top: 6px;">
          {{ settings.camera_password_set === 'true' ? `Gespeichert über ${passwordSource}` : 'Noch kein Kamera-Passwort gespeichert.' }}
        </div>
      </div>
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
  </section>

  <section class="panel card">
    <div class="panel-head">
      <h2>Anzeige</h2>
      <div class="right">Raster und Zuschnitt werden in der Kameras-Ansicht bearbeitet</div>
    </div>
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
        <div class="btn-row"><RouterLink class="btn" to="/">Kameras-Ansicht öffnen</RouterLink></div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useSystem } from '../../composables/useSystem'
import { generalSettingKeys } from '../../composables/settingsDraft'

const { settings, passwordSource, viewerPerformanceOptions, viewerPerformanceDescription, loadAll, saveSettings, saveCameraPassword, setBool, error } = useSystem()
const cameraPassword = ref('')
const savingPassword = ref(false)

async function saveCamPassword() {
  savingPassword.value = true
  try {
    await saveCameraPassword(cameraPassword.value)
    cameraPassword.value = ''
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Passwort konnte nicht gespeichert werden.'
  } finally {
    savingPassword.value = false
  }
}

onMounted(() => void loadAll())
</script>
