<template>
  <AdminDialog ref="dialog" :open="open" title="Kamera umbenennen" :dirty="dirty" :busy="saving" @close="$emit('close')">
    <form class="camera-name-form" @submit.prevent="save">
      <label class="field">
        <span class="lbl">Name</span>
        <input v-model="draft" aria-label="Name" :aria-describedby="hintId" :maxlength="cameraNameMaxLength" :placeholder="fallback" autocomplete="off" autofocus />
      </label>
      <p :id="hintId" class="mono-mute">Erscheint in der Kameraliste, in der Suche und in der Live-Ansicht. Leer lassen, um „{{ fallback }}“ zu zeigen.</p>
      <p v-if="error || validation" class="notice err" role="alert">{{ error || validation }}</p>
      <div class="form-actions">
        <button class="btn" type="button" @click="dialog?.requestClose()">Abbrechen</button>
        <button class="btn primary" type="submit" :disabled="saving || !!validation || !dirty">{{ saving ? 'Speichert…' : 'Name speichern' }}</button>
      </div>
    </form>
  </AdminDialog>
</template>
<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'
import AdminDialog from './AdminDialog.vue'
import { api } from '../api/client'
import { useDraftGuard } from '../composables/discardChanges'
import { cameraHardwareLabel, cameraNameError, cameraNameMaxLength } from '../composables/cameraLabels'
import type { Device } from '../types'
const props = defineProps<{ open: boolean; device?: Device }>()
const emit = defineEmits<{ close: []; saved: [device: Device] }>()
const dialog = ref<InstanceType<typeof AdminDialog>>()
const draft = ref(''), error = ref(''), saving = ref(false), hintId = useId()
const fallback = computed(() => (props.device && cameraHardwareLabel(props.device)) || 'Unbekannte Kamera')
const validation = computed(() => cameraNameError(draft.value))
const dirty = computed(() => props.open && draft.value.trim() !== (props.device?.name || ''))
watch(() => props.open, open => { if (open) { draft.value = props.device?.name || ''; error.value = '' } }, { immediate: true })
useDraftGuard(() => dirty.value, () => emit('close'))
async function save() {
  if (!props.device || saving.value || validation.value) return
  saving.value = true; error.value = ''
  try {
    const device = await api.saveDeviceName(props.device.id, draft.value.trim())
    emit('saved', device); emit('close')
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Name konnte nicht gespeichert werden.'
  } finally { saving.value = false }
}
</script>
<style scoped>
.camera-name-form { display: grid; gap: 16px; }
.camera-name-form p { margin: 0; }
</style>
