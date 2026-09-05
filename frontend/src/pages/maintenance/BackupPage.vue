<template>
  <section class="panel card">
    <div class="panel-head">
      <h2>Sicherung</h2>
      <div class="right">Lokale Konfiguration · Bindings · Einstellungen</div>
    </div>
    <div class="split">
      <div class="field">
        <span class="lbl">Backup erstellen</span>
        <div class="btn-row"><button class="btn primary" :disabled="busy" @click="onBackup">Backup jetzt erstellen</button></div>
      </div>
      <div class="field">
        <span class="lbl">Backup wiederherstellen</span>
        <div class="btn-row" style="align-items: stretch;">
          <input v-model="restorePath" placeholder="/var/lib/camera-appliance/backups/…" style="flex: 1;" />
          <button class="btn" :disabled="busy || !restorePath" @click="onRestore">Wiederherstellen</button>
        </div>
        <div v-if="confirmingRestore" class="notice warn" style="margin-top: 8px;">
          <span class="tag">BESTÄTIGEN</span>
          <div>
            <div>Wirklich dieses Backup wiederherstellen? Aktuelle Einstellungen und Zuordnungen werden ersetzt.</div>
            <div class="btn-row" style="margin-top: 8px;">
              <button class="btn primary" :disabled="busy" @click="doRestore">Ja, wiederherstellen</button>
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
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useSystem } from '../../composables/useSystem'
const { backupResult, error, createBackup, restoreBackup } = useSystem()
const restorePath = ref(''), confirmingRestore = ref(false), busy = ref(false)
async function onBackup() { busy.value=true;try { await createBackup() } catch(err) { error.value=err instanceof Error?err.message:'Backup konnte nicht erstellt werden.' } finally { busy.value=false } }
function onRestore() { confirmingRestore.value=true }
async function doRestore() { confirmingRestore.value=false;busy.value=true;try { await restoreBackup(restorePath.value) } catch(err) { error.value=err instanceof Error?err.message:'Wiederherstellung fehlgeschlagen.' } finally { busy.value=false } }
</script>
