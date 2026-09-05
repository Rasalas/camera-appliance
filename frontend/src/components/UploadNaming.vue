<template>
  <div class="upload-naming">
    <fieldset :disabled="loading || saving || busy" class="naming-fields">
      <label class="mode-label">Dateien
        <select v-model="mode" @change="changeMode">
          <option value="unique">Jedes Mal neu anlegen</option>
          <option value="fixed">Dieselbe Datei ersetzen</option>
        </select>
      </label>
      <label v-if="mode === 'fixed'" class="filename-label"><span class="sr-only">JPEG-Dateiname</span>
        <input v-model="filename" class="input" type="text" placeholder="hof.jpg" maxlength="120" spellcheck="false" autocomplete="off" @input="queueSave" />
      </label>
      <span class="mono-mute naming-status" role="status">{{ loading ? 'Lädt…' : saving ? 'Wird gespeichert…' : dirty ? 'Noch nicht gespeichert' : 'Gespeichert' }}</span>
    </fieldset>
    <p class="mono-mute naming-hint">{{ mode === 'fixed' ? 'Immer das aktuelle Bild unter diesem Namen. Für jede Kamera einen eigenen Namen wählen.' : 'Jeder Upload erhält einen eindeutigen Namen mit Datum und Uhrzeit.' }}</p>
    <p v-if="error" class="notice err" role="alert">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '../api/client'
import type { UploadNaming } from '../types'

const props = defineProps<{ deviceId: string; busy: boolean }>()
const mode = ref<UploadNaming['mode']>('unique')
const filename = ref('')
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const saved = ref('')
const draft = computed<UploadNaming>(() => ({ mode: mode.value, filename: mode.value === 'fixed' ? filename.value : '' }))
const dirty = computed(() => JSON.stringify(draft.value) !== saved.value)
let pending: Promise<boolean> | undefined
let timer: ReturnType<typeof setTimeout> | undefined

function flush(): Promise<boolean> {
  clearTimeout(timer)
  if (pending) return pending
  if (loading.value) return Promise.resolve(false)
  if (mode.value === 'fixed' && !/^[a-z0-9][a-z0-9_.-]*\.(jpg|jpeg)$/i.test(filename.value)) {
    error.value = 'Bitte einen JPEG-Dateinamen eingeben, z. B. hof.jpg. Erlaubt sind A–Z, a–z, 0–9, Punkt, Bindestrich und Unterstrich; keine Verzeichnisse.'
    return Promise.resolve(false)
  }
  error.value = ''
  if (!dirty.value) return Promise.resolve(true)
  saving.value = true
  pending = api.saveUploadNaming(props.deviceId, draft.value).then((result) => {
    saved.value = JSON.stringify(result)
    return true
  }).catch((err) => {
    error.value = err instanceof Error ? err.message : 'Dateiname konnte nicht gespeichert werden.'
    return false
  }).finally(() => { saving.value = false; pending = undefined })
  return pending
}
function queueSave() {
  clearTimeout(timer)
  timer = setTimeout(() => void flush(), 450)
}
function changeMode() {
  clearTimeout(timer)
  error.value = ''
  if (mode.value === 'unique' || filename.value) void flush()
}
onMounted(() => {
  void api.uploadNaming(props.deviceId).then((result) => {
    mode.value = result.mode; filename.value = result.filename
    saved.value = JSON.stringify(draft.value); loading.value = false
  }).catch((err) => { error.value = err instanceof Error ? err.message : 'Dateiname konnte nicht geladen werden.' })
})
onBeforeUnmount(() => { void flush() })
defineExpose({ flush })
</script>

<style scoped>
.upload-naming { display: grid; gap: 8px; border-top: 1px solid var(--line); padding-top: 14px; }
.naming-fields, .mode-label { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.naming-fields { border: 0; padding: 0; margin: 0; }
.mode-label { font-size: 12px; }
.mode-label select { width: auto; }
.filename-label { flex: 1; min-width: 140px; }
.filename-label input { width: 100%; }
.naming-status, .naming-hint { font-size: 11px; }
.naming-status { margin-left: auto; }
.naming-hint { margin: 0; line-height: 1.5; }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
</style>
