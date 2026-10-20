<template>
  <SettingsForm title="Überwachung" :setting-keys="maintenanceSettingKeys">
    <template #summary><dl class="spec"><div><dt>Status</dt><dd>{{ watchdogEnabled ? 'Aktiv' : 'Aus' }}</dd></div><div><dt>Schneller Check</dt><dd>{{ settings['watchdog.fast_interval_seconds'] }} Sekunden</dd></div><div><dt>Verbindungswege prüfen</dt><dd>{{ settings['watchdog.camera_interval_seconds'] }} Sekunden</dd></div><div><dt>Letzte Aktion</dt><dd>{{ status?.watchdog?.last_action || 'Noch keine Aktion.' }}</dd></div></dl></template>
    <div class="toggle-stack">
      <label class="toggle-row">
        <input type="checkbox" :checked="watchdogEnabled" @change="setBool('watchdog.enabled', $event)" />
        <div><div class="lbl-main">Überwachung aktiv</div><div class="lbl-sub">Prüft im Hintergrund den Videodienst, die Verbindungswege der Kameras und die Weiterleitungen.</div></div>
      </label>
      <label class="toggle-row">
        <input type="checkbox" :checked="watchdogRestartOnChange" @change="setBool('watchdog.restart_on_change', $event)" />
        <div><div class="lbl-main">Videodienst bei Wechsel des Verbindungswegs neu starten</div><div class="lbl-sub">Ein automatischer Wechsel wirkt sofort in der Live-Ansicht.</div></div>
      </label>
      <label class="toggle-row">
        <input type="checkbox" :checked="watchdogRestartGo2RTC" @change="setBool('watchdog.restart_go2rtc_on_failure', $event)" />
        <div><div class="lbl-main">Videodienst bei Ausfall neu starten</div><div class="lbl-sub">Wenn der Videodienst nicht antwortet, wird er automatisch neu gestartet.</div></div>
      </label>
    </div>

    <div class="split">
      <div class="field"><span class="lbl">Schneller Check · Sekunden</span><input aria-label="Schneller Check · Sekunden" v-model="settings['watchdog.fast_interval_seconds']" type="number" min="5" max="3600" /></div>
      <div class="field"><span class="lbl">Verbindungswege prüfen · Sekunden</span><input aria-label="Verbindungswege prüfen · Sekunden" v-model="settings['watchdog.camera_interval_seconds']" type="number" min="10" max="7200" /></div>
      <div class="field"><span class="lbl">Fehler bis Wechsel</span><input aria-label="Fehler bis Wechsel" v-model="settings['camera.path.fail_threshold']" type="number" min="1" max="20" /></div>
      <div class="field"><span class="lbl">Erfolge bis Rückwechsel</span><input aria-label="Erfolge bis Rückwechsel" v-model="settings['camera.path.recovery_threshold']" type="number" min="1" max="20" /></div>
      <div class="field"><span class="lbl">Wartezeit zwischen Neustarts · Sekunden</span><input aria-label="Wartezeit zwischen Neustarts · Sekunden" v-model="settings['camera.path.restart_cooldown_seconds']" type="number" min="0" max="7200" /></div>
    </div>

    <dl class="spec">
      <div><dt>Letzter Lauf</dt><dd>{{ watchdogDate(status?.watchdog?.last_run_at) }}</dd></div>
      <div><dt>Nächster Lauf</dt><dd>{{ watchdogDate(status?.watchdog?.next_run_at) }}</dd></div>
      <div><dt>Letzte Aktion</dt><dd>{{ status?.watchdog?.last_action || 'Noch keine Aktion.' }}</dd></div>
      <div><dt>Letzter Fehler</dt><dd>{{ status?.watchdog?.last_error || 'Kein Fehler.' }}</dd></div>
      <div><dt>Wartezeit zwischen Neustarts</dt><dd>{{ restartCooldownLabel }}</dd></div>
    </dl>
  </SettingsForm>
</template>

<script setup lang="ts">
import SettingsForm from '../../components/SettingsForm.vue'
import { useSystem } from '../../composables/useSystem'
import { maintenanceSettingKeys } from '../../composables/settingsDraft'
const { settings, status, watchdogEnabled, watchdogRestartOnChange, watchdogRestartGo2RTC, restartCooldownLabel, setBool, watchdogDate } = useSystem()
</script>

<style scoped>
.toggle-stack { display: grid; gap: 8px; }
</style>
