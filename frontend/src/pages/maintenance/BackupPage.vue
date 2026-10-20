<template>
  <section class="panel card backup-section">
    <div class="panel-head">
      <h2>Sicherungen</h2>
      <div class="right">Kameras · Namen · Einstellungen</div>
    </div>
    <div class="notice warn">
      <span class="tag">WICHTIG</span>
      <div>
        <div>{{ list?.warning || 'Enthält Kamera-Passwörter – sicher aufbewahren.' }}</div>
        <div class="mono-mute backup-hint">Lade eine Sicherung herunter und kopiere sie auf ein anderes Gerät oder einen USB-Stick. Fällt diese Station aus, sind die Sicherungen auf ihr ebenfalls weg.</div>
      </div>
    </div>
    <div class="btn-row">
      <button class="btn primary" :disabled="creating" @click="onBackup">{{ creating ? 'Sicherung läuft…' : 'Jetzt sichern' }}</button>
    </div>
    <label class="toggle-row">
      <input type="checkbox" :checked="list?.auto.enabled ?? true" :disabled="!list || savingAuto" @change="onAutoChange" />
      <div><div class="lbl-main">Täglich automatisch sichern</div><div class="lbl-sub">{{ autoBackupSummary(list?.auto) }} Sicherungen von Hand und vor Updates werden nie automatisch gelöscht.</div></div>
    </label>
    <p v-if="list?.auto.last_error" class="notice err" role="alert">Letzte automatische Sicherung fehlgeschlagen: {{ list.auto.last_error }}</p>

    <p v-if="loading && !list" role="status" class="mono-mute">Sicherungen werden geladen…</p>
    <div v-else-if="loadError" class="notice err" role="alert">{{ loadError }}<button class="btn ghost" @click="load">Erneut versuchen</button></div>
    <p v-else-if="!list?.backups.length" class="mono-mute">Noch keine Sicherungen vorhanden.</p>
    <ul v-else class="backup-list" aria-label="Vorhandene Sicherungen">
      <li v-for="entry in list.backups" :key="entry.name">
        <div class="backup-meta">
          <strong><time :datetime="entry.created_at">{{ backupTimeLabel(entry.created_at) }}</time></strong>
          <span class="mono-mute">{{ backupKindLabel(entry.kind) }} · {{ backupSizeLabel(entry.size) }}</span>
        </div>
        <div class="backup-actions">
          <button class="btn sm" :disabled="downloading === entry.name" :aria-label="`Sicherung vom ${backupTimeLabel(entry.created_at)} herunterladen`" @click="onDownload(entry)"><AppIcon name="download" />{{ downloading === entry.name ? 'Lädt…' : 'Herunterladen' }}</button>
          <button class="btn sm danger" :aria-label="`Sicherung vom ${backupTimeLabel(entry.created_at)} löschen`" @click="deleting = entry">Löschen</button>
        </div>
      </li>
    </ul>
    <p v-if="actionError" class="notice err" role="alert">{{ actionError }}</p>
    <p v-if="downloadMessage" role="status" class="mono-mute">{{ downloadMessage }}</p>
  </section>

  <section class="panel card">
    <div class="panel-head"><h2>Wiederherstellen</h2></div>
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
    <div v-if="backupResult" class="notice ok">
      <span class="tag">FERTIG</span>
      <div>
        <div>{{ backupResult.path }}</div>
        <div v-if="backupResult.warning" class="mono-mute result-warning">{{ backupResult.warning }}</div>
      </div>
    </div>
  </section>

  <AdminDialog :open="!!deleting" title="Sicherung löschen?" compact :busy="removing" @close="deleting = undefined">
    <p>Die Sicherung vom {{ deleting ? backupTimeLabel(deleting.created_at) : '' }} wird von der Station gelöscht. Das lässt sich nicht rückgängig machen.</p>
    <p v-if="deleteError" class="notice err" role="alert">{{ deleteError }}</p>
    <div class="form-actions">
      <button class="btn" :disabled="removing" @click="deleting = undefined">Abbrechen</button>
      <button class="btn danger" :disabled="removing" @click="onDelete">Löschen</button>
    </div>
  </AdminDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api } from '../../api/client'
import { useSystem } from '../../composables/useSystem'
import { autoBackupSummary, backupKindLabel, backupSizeLabel, backupTimeLabel } from '../../composables/backupList'
import type { BackupEntry, BackupList } from '../../types'
import AdminDialog from '../../components/AdminDialog.vue'
import AppIcon from '../../components/AppIcon.vue'
const { backupResult, error, createBackup, restoreBackup, showToast } = useSystem()
const list = ref<BackupList>(), loading = ref(true), loadError = ref('')
const creating = ref(false), savingAuto = ref(false), downloading = ref(''), actionError = ref(''), downloadMessage = ref('')
const deleting = ref<BackupEntry>(), removing = ref(false), deleteError = ref('')
const restorePath = ref(''), restoreFile = ref<File>(), pending = ref<File | string>(), busy = ref(false), restoring = ref(false)
const fileInput = ref<HTMLInputElement>()
const pendingName = computed(() => typeof pending.value === 'string' ? pending.value : pending.value?.name ?? '')
let downloadURL = ''
watch(deleting, () => { deleteError.value = '' })

async function load() {
  loading.value = true; loadError.value = ''
  try { list.value = await api.backups() }
  catch (err) { loadError.value = err instanceof Error ? err.message : 'Sicherungen konnten nicht geladen werden.' }
  finally { loading.value = false }
}
async function onBackup() {
  creating.value = true; actionError.value = ''
  try { await createBackup(); await load() }
  catch (err) { actionError.value = err instanceof Error ? err.message : 'Sicherung konnte nicht erstellt werden.' }
  finally { creating.value = false }
}
async function onAutoChange(event: Event) {
  const input = event.target as HTMLInputElement
  savingAuto.value = true; actionError.value = ''
  try {
    await api.saveSettings({ 'backup.auto.enabled': String(input.checked) })
    showToast(input.checked ? 'Automatische Sicherung eingeschaltet' : 'Automatische Sicherung ausgeschaltet')
    await load()
  } catch (err) {
    input.checked = !input.checked
    actionError.value = err instanceof Error ? err.message : 'Einstellung konnte nicht gespeichert werden.'
  } finally { savingAuto.value = false }
}
async function onDownload(entry: BackupEntry) {
  downloading.value = entry.name; actionError.value = ''; downloadMessage.value = ''
  try {
    const blob = await api.downloadBackup(entry.name)
    if (downloadURL) URL.revokeObjectURL(downloadURL)
    downloadURL = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = downloadURL; link.download = entry.name
    document.body.append(link); link.click(); link.remove()
    downloadMessage.value = 'Download gestartet. Kopiere die Datei danach auf ein anderes Gerät oder einen USB-Stick.'
  } catch (err) { actionError.value = err instanceof Error ? err.message : 'Download fehlgeschlagen.' }
  finally { downloading.value = '' }
}
async function onDelete() {
  const entry = deleting.value
  if (!entry) return
  removing.value = true; deleteError.value = ''
  try {
    await api.deleteBackup(entry.name)
    deleting.value = undefined
    showToast('Sicherung gelöscht')
    await load()
  } catch (err) { deleteError.value = err instanceof Error ? err.message : 'Löschen fehlgeschlagen.' }
  finally { removing.value = false }
}

function onFile(event: Event) { restoreFile.value = (event.target as HTMLInputElement).files?.[0] }
function confirm(source: File | string | undefined) { if (source) pending.value = source }
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
onMounted(load)
onBeforeUnmount(() => { if (downloadURL) URL.revokeObjectURL(downloadURL) })
</script>

<style scoped>
.backup-section { display:grid;gap:18px; }
.backup-section > p { margin:0; }
.notice .tag { flex-shrink:0; }
.backup-hint, .restore-confirm, .result-warning { margin-top:8px; }
.backup-list { list-style:none;margin:0;padding:0; }
.backup-list li { display:flex;align-items:center;justify-content:space-between;gap:12px;padding:12px 8px;flex-wrap:wrap; }
.backup-list li:nth-child(odd) { background:rgba(255,255,255,.018);border-radius:var(--radius-sm); }
.backup-meta { display:grid;gap:4px;min-width:0; }
.backup-meta strong { font-weight:500; }
.backup-actions { display:flex;gap:8px;flex-wrap:wrap; }
.restore-path { align-items:stretch;margin-top:8px; }
.restore-path input { flex:1; }
.restore-confirm .btn-row { margin-top:8px; }
</style>
