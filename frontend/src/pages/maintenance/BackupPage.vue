<template>
  <section class="panel card">
    <div class="panel-head">
      <h2>Sicherung</h2>
      <div class="right">Kameras · Namen · Einstellungen</div>
    </div>
    <div class="split">
      <div class="field">
        <span class="lbl">Sicherung erstellen</span>
        <div class="btn-row"><button class="btn primary" :disabled="busy" @click="onBackup">Sicherung jetzt erstellen</button></div>
      </div>
      <div class="field">
        <label class="lbl" for="restore-file">Sicherung wiederherstellen</label>
        <input id="restore-file" ref="fileInput" type="file" accept=".tar.gz,.tgz,application/gzip" :disabled="busy" @change="onFile" />
        <span class="mono-mute">Wähle eine Sicherungsdatei (.tar.gz) von diesem Gerät oder einem USB-Stick.</span>
        <div class="btn-row"><button class="btn" :disabled="busy || !restoreFile" @click="confirm(restoreFile)">Wiederherstellen</button></div>
        <details class="advanced">
          <summary>Erweitert: Datei auf der Station</summary>
          <div class="btn-row restore-path">
            <input v-model="restorePath" aria-label="Pfad der Sicherungsdatei auf der Station" placeholder="/var/lib/camera-appliance/backups/…" />
            <button class="btn" :disabled="busy || !restorePath.trim()" @click="confirm(restorePath.trim())">Wiederherstellen</button>
          </div>
        </details>
        <div v-if="pending" class="notice warn restore-confirm">
          <span class="tag">BESTÄTIGEN</span>
          <div>
            <div>Wirklich „{{ pendingName }}“ wiederherstellen? Aktuelle Einstellungen und Zuordnungen werden ersetzt.</div>
            <div class="btn-row">
              <button class="btn primary" :disabled="busy" @click="doRestore">Ja, wiederherstellen</button>
              <button class="btn" :disabled="busy" @click="pending = undefined">Abbrechen</button>
            </div>
          </div>
        </div>
        <div v-if="busy && restoring" class="progress" role="status" aria-label="Wiederherstellung läuft" />
      </div>
    </div>
    <div v-if="backupResult" class="notice ok">
      <span class="tag">FERTIG</span>
      <div>
        <div>{{ backupResult.path }}</div>
        <div v-if="backupResult.warning" class="mono-mute result-warning">{{ backupResult.warning }}</div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useSystem } from '../../composables/useSystem'
const { backupResult, error, createBackup, restoreBackup } = useSystem()
const restorePath = ref(''), restoreFile = ref<File>(), pending = ref<File | string>(), busy = ref(false), restoring = ref(false)
const fileInput = ref<HTMLInputElement>()
const pendingName = computed(() => typeof pending.value === 'string' ? pending.value : pending.value?.name ?? '')
function onFile(event: Event) { restoreFile.value = (event.target as HTMLInputElement).files?.[0] }
function confirm(source: File | string | undefined) { if (source) pending.value = source }
async function onBackup() { busy.value=true;try { await createBackup() } catch(err) { error.value=err instanceof Error?err.message:'Sicherung konnte nicht erstellt werden.' } finally { busy.value=false } }
async function doRestore() {
  const source = pending.value
  if (!source) return
  busy.value = true; restoring.value = true; error.value = ''
  try {
    await restoreBackup(source)
    pending.value = undefined; restoreFile.value = undefined
    if (fileInput.value) fileInput.value.value = ''
  } catch (err) { error.value = err instanceof Error ? err.message : 'Wiederherstellung fehlgeschlagen.' }
  finally { busy.value = false; restoring.value = false }
}
</script>

<style scoped>
.restore-path { align-items: stretch; margin-top: 8px; }
.restore-path input { flex: 1; }
.restore-confirm, .result-warning { margin-top: 8px; }
.restore-confirm .btn-row { margin-top: 8px; }
</style>
