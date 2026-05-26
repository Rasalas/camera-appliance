<template>
  <PageHeader title="Backup" subtitle="Lokale Appliance-Konfiguration sichern und wiederherstellen" />
  <ErrorMessage :message="error" />
  <Card>
    <p>Erstelle ein Backup der lokalen Appliance-Konfiguration.</p>
    <div class="grid two compact">
      <div>
        <h3>Enthalten</h3>
        <ul><li>Kamera-Zuordnungen</li><li>Anzeigenamen</li><li>lokale Einstellungen</li><li>generierte go2rtc-Konfiguration</li></ul>
      </div>
      <div>
        <h3>Nicht enthalten</h3>
        <ul><li>Docker Images</li><li>Git Repository</li><li>Videoaufzeichnungen</li></ul>
      </div>
    </div>
    <div class="button-row">
      <button class="action-button primary" @click="create">Backup erstellen</button>
    </div>
  </Card>
  <Card>
    <h2>Backup wiederherstellen</h2>
    <label>Pfad zur Backup-Datei<input v-model="restorePath" placeholder="/var/lib/camera-appliance/backups/..." /></label>
    <button class="action-button secondary" @click="restore">Backup wiederherstellen</button>
  </Card>
  <Card v-if="result">
    <h2>Ergebnis</h2>
    <p>{{ result.warning }}</p>
    <p>{{ result.path }}</p>
  </Card>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { api } from '../api/client'
import PageHeader from '../components/PageHeader.vue'
import Card from '../components/Card.vue'
import ErrorMessage from '../components/ErrorMessage.vue'

const restorePath = ref('')
const error = ref('')
const result = ref<{ path: string; warning: string }>()

async function create() {
  error.value = ''
  try {
    result.value = await api.backup()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Backup konnte nicht erstellt werden.'
  }
}

async function restore() {
  error.value = ''
  try {
    result.value = await api.restore(restorePath.value)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Backup konnte nicht wiederhergestellt werden.'
  }
}
</script>
