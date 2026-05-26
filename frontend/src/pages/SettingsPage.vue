<template>
  <PageHeader title="Einstellungen" subtitle="Lokale Appliance-Einstellungen" />
  <ErrorMessage :message="error" />
  <Card>
    <label>Kamera-Passwort<input :value="settings.camera_password_set === 'true' ? '***************' : 'nicht gesetzt'" disabled /></label>
    <div class="checkboxes">
      <label><input v-model="settings.auto_discover" type="checkbox" true-value="true" false-value="false" /> Beim Start automatisch suchen</label>
      <label><input v-model="settings.render_after_discovery" type="checkbox" true-value="true" false-value="false" /> go2rtc-Konfiguration nach erfolgreicher Suche erzeugen</label>
      <label><input v-model="settings.restart_after_render" type="checkbox" true-value="true" false-value="false" /> go2rtc nach Änderungen neu starten</label>
    </div>
    <p>AgentDVR URL: {{ settings.agentdvr_url }}</p>
    <p>go2rtc URL: {{ settings.go2rtc_url }}</p>
    <p>Admin-Oberfläche: {{ settings.bind_addr }}</p>
    <button class="action-button primary" @click="save">Einstellungen speichern</button>
  </Card>
  <Toast :message="toast" />
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api } from '../api/client'
import PageHeader from '../components/PageHeader.vue'
import Card from '../components/Card.vue'
import ErrorMessage from '../components/ErrorMessage.vue'
import Toast from '../components/Toast.vue'

const settings = reactive<Record<string, string>>({})
const error = ref('')
const toast = ref('')

async function save() {
  try {
    await api.saveSettings(settings)
    toast.value = 'Einstellungen gespeichert'
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Einstellungen konnten nicht gespeichert werden.'
  }
}

onMounted(async () => Object.assign(settings, await api.settings()))
</script>
