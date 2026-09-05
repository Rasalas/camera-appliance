<template>
  <section class="panel card">
    <div class="panel-head">
      <h2>Watchdog</h2>
      <button class="btn sm primary" @click="saveSettings(maintenanceSettingKeys)">Speichern</button>
    </div>

    <div style="display: grid; gap: 8px;">
      <label class="toggle-row">
        <input type="checkbox" :checked="watchdogEnabled" @change="setBool('watchdog.enabled', $event)" />
        <div><div class="lbl-main">Watchdog aktiv</div><div class="lbl-sub">Prüft go2rtc, aktive Kamera-Pfade und Relay-Fallbacks im Hintergrund.</div></div>
      </label>
      <label class="toggle-row">
        <input type="checkbox" :checked="watchdogRestartOnChange" @change="setBool('watchdog.restart_on_change', $event)" />
        <div><div class="lbl-main">go2rtc bei Pfadwechsel neu starten</div><div class="lbl-sub">Automatische Pfadwechsel werden direkt in den Streams wirksam.</div></div>
      </label>
      <label class="toggle-row">
        <input type="checkbox" :checked="watchdogRestartGo2RTC" @change="setBool('watchdog.restart_go2rtc_on_failure', $event)" />
        <div><div class="lbl-main">go2rtc bei Ausfall neu starten</div><div class="lbl-sub">Wenn die go2rtc-API nicht erreichbar ist, versucht der Watchdog einen Neustart.</div></div>
      </label>
    </div>

    <div class="split">
      <div class="field"><span class="lbl">Schneller Check · Sekunden</span><input v-model="settings['watchdog.fast_interval_seconds']" type="number" min="5" max="3600" /></div>
      <div class="field"><span class="lbl">Kamera-Pfade · Sekunden</span><input v-model="settings['watchdog.camera_interval_seconds']" type="number" min="10" max="7200" /></div>
      <div class="field"><span class="lbl">Fehler bis Wechsel</span><input v-model="settings['camera.path.fail_threshold']" type="number" min="1" max="20" /></div>
      <div class="field"><span class="lbl">Erfolge bis Rückwechsel</span><input v-model="settings['camera.path.recovery_threshold']" type="number" min="1" max="20" /></div>
      <div class="field"><span class="lbl">Restart-Cooldown · Sekunden</span><input v-model="settings['camera.path.restart_cooldown_seconds']" type="number" min="0" max="7200" /></div>
    </div>

    <dl class="spec">
      <div><dt>Letzter Lauf</dt><dd>{{ watchdogDate(status?.watchdog?.last_run_at) }}</dd></div>
      <div><dt>Nächster Lauf</dt><dd>{{ watchdogDate(status?.watchdog?.next_run_at) }}</dd></div>
      <div><dt>Letzte Aktion</dt><dd>{{ status?.watchdog?.last_action || 'Noch keine Aktion.' }}</dd></div>
      <div><dt>Letzter Fehler</dt><dd>{{ status?.watchdog?.last_error || 'Kein Fehler.' }}</dd></div>
      <div><dt>Restart-Cooldown</dt><dd>{{ restartCooldownLabel }}</dd></div>
    </dl>
  </section>

  <section class="panel card">
    <div class="panel-head">
      <h2>Sicherung</h2>
      <div class="right">Lokale Konfiguration · Bindings · Einstellungen</div>
    </div>
    <div class="split">
      <div class="field">
        <span class="lbl">Backup erstellen</span>
        <div class="btn-row"><button class="btn primary" @click="onBackup">Backup jetzt erstellen</button></div>
      </div>
      <div class="field">
        <span class="lbl">Backup wiederherstellen</span>
        <div class="btn-row" style="align-items: stretch;">
          <input v-model="restorePath" placeholder="/var/lib/camera-appliance/backups/…" style="flex: 1;" />
          <button class="btn" :disabled="!restorePath" @click="onRestore">Wiederherstellen</button>
        </div>
        <div v-if="confirmingRestore" class="notice warn" style="margin-top: 8px;">
          <span class="tag">BESTÄTIGEN</span>
          <div>
            <div>Wirklich dieses Backup wiederherstellen? Aktuelle Einstellungen und Zuordnungen werden ersetzt.</div>
            <div class="btn-row" style="margin-top: 8px;">
              <button class="btn primary" @click="doRestore">Ja, wiederherstellen</button>
              <button class="btn" @click="confirmingRestore = false">Abbrechen</button>
            </div>
          </div>
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

  <section class="panel card">
    <div class="panel-head">
      <h2>Version & Updates</h2>
      <div class="right">Release · Backup · Neustart</div>
    </div>
    <div class="split">
      <div class="field">
        <span class="lbl">Installierte Version</span>
        <div class="mono-mute">{{ versionDetail }}</div>
      </div>
      <div class="field">
        <span class="lbl">Release-URL (optional, sonst GitHub Latest)</span>
        <input v-model="updateUrl" class="input mono" type="text" placeholder="http://…/camera-appliance-latest.tar.gz" spellcheck="false" />
      </div>
      <div class="field">
        <span class="lbl">SHA-256-Prüfsumme (optional)</span>
        <input v-model="updateDigest" class="input mono" type="text" placeholder="sha256:…" spellcheck="false" />
      </div>
      <div class="field">
        <span class="lbl">Update</span>
        <div class="btn-row"><button class="btn primary" :disabled="updating" @click="onUpdate">{{ updating ? 'Update läuft…' : 'Update installieren' }}</button></div>
      </div>
    </div>
    <div v-if="updateResult" class="notice warn">
      <span class="tag">UPDATE</span>
      <div>
        <div>Update wurde gestartet. Die Oberfläche kann während Build und Neustart kurz nicht erreichbar sein.</div>
        <div class="mono-mute" style="margin-top: 4px;">{{ updateResult.url }}</div>
      </div>
    </div>
  </section>

  <section class="panel card">
    <div class="panel-head">
      <h2>Support-Bundle</h2>
      <div class="right">Status · Viewer · Netzwerk · Logs</div>
    </div>
    <div class="split">
      <div class="field">
        <span class="lbl">Diagnosepaket</span>
        <div class="btn-row"><button class="btn primary" :disabled="creatingSupportBundle" @click="onSupportBundle">{{ creatingSupportBundle ? 'Erstellt…' : 'Support-Bundle erstellen' }}</button></div>
      </div>
    </div>
    <div v-if="supportBundleResult" class="notice ok">
      <span class="tag">FERTIG</span>
      <div class="support-result">
        <div>{{ supportBundleResult.path }}</div>
        <div class="mono-mute">{{ supportBundleResult.files.length }} Dateien · Zugangsdaten maskiert</div>
        <div v-if="supportBundleResult.warning" class="mono-mute">{{ supportBundleResult.warning }}</div>
      </div>
    </div>
  </section>

  <section class="panel card">
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
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useSystem } from '../../composables/useSystem'
import { maintenanceSettingKeys } from '../../composables/settingsDraft'

const {
  settings, status, events, backupResult, supportBundleResult, updateResult, error,
  watchdogEnabled, watchdogRestartOnChange, watchdogRestartGo2RTC, restartCooldownLabel, versionDetail,
  loadAll, refreshStatus, saveSettings, createBackup, restoreBackup, createSupportBundle, startUpdate,
  setBool, watchdogDate, formatTime, levelClass
} = useSystem()

const restorePath = ref('')
const confirmingRestore = ref(false)
const updateDigest = ref('')
const updateUrl = ref('')
const creatingSupportBundle = ref(false)
const updating = ref(false)

async function onBackup() {
  try { await createBackup() } catch (err) { error.value = err instanceof Error ? err.message : 'Backup konnte nicht erstellt werden.' }
}
async function onRestore() {
  confirmingRestore.value = true
}
async function doRestore() {
  confirmingRestore.value = false
  try {
    await restoreBackup(restorePath.value)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Wiederherstellung fehlgeschlagen.'
  }
}
async function onSupportBundle() {
  creatingSupportBundle.value = true
  try { await createSupportBundle() } catch (err) { error.value = err instanceof Error ? err.message : 'Support-Bundle konnte nicht erstellt werden.' } finally { creatingSupportBundle.value = false }
}
async function onUpdate() {
  updating.value = true
  try {
    await startUpdate(updateUrl.value.trim() || undefined, updateDigest.value.trim() || undefined)
    window.setTimeout(() => void refreshStatus().finally(() => { updating.value = false }), 20000)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Update konnte nicht gestartet werden.'
    updating.value = false
  }
}

onMounted(() => void loadAll())
</script>
