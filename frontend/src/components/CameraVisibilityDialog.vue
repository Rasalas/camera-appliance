<template>
  <AdminDialog :open="open" :title="shown ? 'Kamera ausblenden?' : 'Kamera einblenden?'" :busy="busy" compact @close="$emit('close')">
    <p>{{ shown ? `„${name}“ wird aus der Live-Ansicht entfernt.` : `„${name}“ wird in der Live-Ansicht angezeigt.` }}</p>
    <p class="mono-mute">Die Live-Ansicht verbindet sich dafür kurz neu. Alle Kamerabilder sind dabei einige Sekunden nicht zu sehen.</p>
    <p v-if="error" class="notice err" role="alert">{{ error }}</p>
    <div class="form-actions">
      <button class="btn" type="button" :disabled="busy" @click="$emit('close')">Abbrechen</button>
      <button class="btn" :class="shown ? 'danger' : 'primary'" type="button" :disabled="busy" @click="$emit('confirm')">{{ busy ? 'Wird geändert…' : shown ? 'Ausblenden' : 'Einblenden' }}</button>
    </div>
  </AdminDialog>
</template>
<script setup lang="ts">
import AdminDialog from './AdminDialog.vue'
defineProps<{ open: boolean; name: string; shown: boolean; busy?: boolean; error?: string }>()
defineEmits<{ close: []; confirm: [] }>()
</script>
