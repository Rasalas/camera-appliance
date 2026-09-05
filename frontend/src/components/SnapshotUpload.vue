<template>
  <section class="panel card snapshot-editor">
    <div class="panel-head">
      <h2>Bild-Upload</h2>
      <RouterLink class="server-link mono-mute" to="/kameras/bild-upload">Server einstellen</RouterLink>
    </div>
    <div v-if="settingsError || saveError" class="notice err" role="alert">{{ settingsError || saveError }}</div>
    <UploadImageEditor ref="imageEditor" v-model:crop="crop" :device-id="deviceId" :camera-label="cameraLabel" :src="previewSrc" :captured-at="capturedAt" :busy="uploading" :crop-loading="cropLoading" :crop-status="saveStatus" :preview-loading="previewLoading" :preview-error="previewError" @selecting="selectionChanged" />
    <div v-if="previewError" class="preview-error mono-mute" role="alert">{{ previewSrc ? previewError : '' }} <button v-if="canCapture" class="retry-link" type="button" :disabled="previewLoading" @click="loadPreview">Erneut laden</button></div>
    <div class="btn-row"><span class="mono-mute">Der Ausschnitt wird automatisch gespeichert.</span><button class="btn ghost" :disabled="cropLoading || uploading || !cropChanged" @click="restoreCrop">Ausschnitt seit Öffnen zurücknehmen</button></div>
    <UploadNaming ref="naming" :device-id="deviceId" :busy="uploading" :default-directory="destination?.directory || '.'" />
    <UploadSchedule :device-id="deviceId" :before-enable="prepareSchedule" />
    <div class="upload-footer">
      <span class="mono-mute">{{ destination?.password_set ? `${destination.protocol.toUpperCase()} · ${destination.host}` : 'Upload-Server einrichten' }}</span>
      <button class="btn primary" type="button" :disabled="cropLoading || previewLoading || uploading || selecting || cameraBusy || !validCrop || !canCapture || !destination?.password_set" @click="upload">{{ uploading ? 'Wird hochgeladen…' : 'Jetzt hochladen' }}</button>
    </div>
    <div v-if="uploadError" class="notice err" role="alert">{{ uploadError }}</div>
    <div v-if="uploadMessage" class="notice ok" role="status">{{ uploadMessage }}</div>
    <details v-if="crop.enabled" class="advanced">
      <summary>Genaue Werte</summary>
      <fieldset class="crop-inputs" :disabled="cropLoading || uploading">
        <label class="field"><span class="lbl">Links %</span><input aria-label="Links %" v-model.number="crop.x" type="number" min="0" max="99" step="0.1" /></label>
        <label class="field"><span class="lbl">Oben %</span><input aria-label="Oben %" v-model.number="crop.y" type="number" min="0" max="99" step="0.1" /></label>
        <label class="field"><span class="lbl">Breite %</span><input aria-label="Breite %" v-model.number="crop.width" type="number" min="0.1" max="100" step="0.1" /></label>
        <label class="field"><span class="lbl">Höhe %</span><input aria-label="Höhe %" v-model.number="crop.height" type="number" min="0.1" max="100" step="0.1" /></label>
      </fieldset>
    </details>
    <p v-if="!validCrop" class="notice err" role="alert">Der Ausschnitt muss innerhalb des Bildes liegen und größer als 0 sein.</p>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api } from '../api/client'
import UploadSchedule from './UploadSchedule.vue'
import UploadNaming from './UploadNaming.vue'
import UploadImageEditor from './UploadImageEditor.vue'
import { createCropAutosave, validUploadCrop } from '../composables/uploadCropDraft'
import type { UploadCrop, UploadSettings } from '../types'

const props = defineProps<{ deviceId: string; cameraLabel: string; imageSrc: string; username: string; password: string; stream: string; canCapture: boolean; cameraBusy: boolean }>()
const crop = ref<UploadCrop>({ enabled: false, x: 0, y: 0, width: 100, height: 100 })
const originalCrop=ref<UploadCrop>()
const cropChanged=computed(()=>!!originalCrop.value && JSON.stringify(crop.value)!==JSON.stringify(originalCrop.value))
function restoreCrop() { if(originalCrop.value) {crop.value={...originalCrop.value};void autosave.flush()} }
const destination = ref<UploadSettings>()
const cropLoading = ref(true)
const uploading = ref(false)
const naming = ref<InstanceType<typeof UploadNaming>>()
const settingsError = ref('')
const saveError = ref('')
const saveStatus = ref('')
const uploadError = ref('')
const uploadMessage = ref('')
const previewSrc = ref(props.imageSrc)
const previewLoading = ref(false)
const previewError = ref('')
const capturedAt = ref('')
const imageEditor = ref<InstanceType<typeof UploadImageEditor>>()
const selecting = ref(false)
const validCrop = computed(() => validUploadCrop(crop.value))
const autosave = createCropAutosave(
  (draft) => api.saveUploadCrop(props.deviceId, draft),
  (state, err) => {
    saveStatus.value = { pending: 'Wird gespeichert…', saving: 'Wird gespeichert…', saved: 'Gespeichert', error: 'Nicht gespeichert' }[state]
    saveError.value = state === 'error' ? (err instanceof Error ? err.message : 'Ausschnitt konnte nicht gespeichert werden.') : ''
  }
)

watch(crop, scheduleSave, { deep: true, flush: 'sync' })

function scheduleSave() {
  if (cropLoading.value || selecting.value) return
  if (!validCrop.value) { autosave.cancelPending(); saveStatus.value = 'Nicht gespeichert'; return }
  autosave.change(crop.value)
}
async function prepareSchedule() {
  if (cropLoading.value || !validCrop.value || selecting.value) return false
  await autosave.flush()
  return !saveError.value && !!await imageEditor.value?.flush() && !!await naming.value?.flush()
}
function selectionChanged(active: boolean) {
  selecting.value = active
  if (active) autosave.cancelPending()
  else scheduleSave()
}
async function loadPreview() {
  if (!props.canCapture) { previewError.value = 'Bitte zuerst den Kamerazugang einrichten.'; return }
  previewLoading.value = true; previewError.value = ''
  try {
    const frame = await api.captureFrame(props.deviceId, { username: props.username, password: props.password, stream: props.stream })
    previewSrc.value = `data:${frame.content_type};base64,${frame.image_base64}`
    capturedAt.value = frame.captured_at
  } catch (err) { previewError.value = err instanceof Error ? err.message : 'Vorschau konnte nicht geladen werden.' }
  finally { previewLoading.value = false }
}
async function upload() {
  uploading.value = true; uploadError.value = ''; uploadMessage.value = ''
  try {
    if (!await imageEditor.value?.flush()) throw new Error('Privatbereiche und Zeitangabe müssen fehlerfrei gespeichert sein. Upload abgebrochen.')
    if (!await naming.value?.flush()) throw new Error('Bitte zuerst Dateiname und Verzeichnis prüfen und speichern lassen.')
    const result = await api.uploadSnapshot(props.deviceId, { username: props.username, password: props.password, stream: props.stream, crop: { ...crop.value } })
    uploadMessage.value = `Hochgeladen · ${result.filename} · ${result.width} × ${result.height} Pixel`
  } catch (err) { uploadError.value = err instanceof Error ? err.message : 'Bild konnte nicht hochgeladen werden.' }
  finally { uploading.value = false }
}
onMounted(() => {
  void api.uploadCrop(props.deviceId).then((saved) => { crop.value = saved; originalCrop.value={...saved}; cropLoading.value = false }).catch((err) => { settingsError.value = err instanceof Error ? err.message : 'Ausschnitt konnte nicht geladen werden.' })
  void api.uploadSettings().then((settings) => { destination.value = settings }).catch((err) => { settingsError.value = err instanceof Error ? err.message : 'Upload-Server konnte nicht geladen werden.' })
  void loadPreview()
})
onBeforeUnmount(() => { void autosave.close() })
</script>

<style scoped>
.snapshot-editor { gap: 16px; }
.notice { overflow-wrap: anywhere; }
.server-link { font-size: 11px; }
.upload-footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.crop-inputs { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; border: 0; margin: 12px 0 0; padding: 0; }
.retry-link { border: 0; padding: 0; color: var(--ink); background: transparent; text-decoration: underline; cursor: pointer; }
.preview-error, .upload-footer > span { font-size: 11px; }
@media (max-width: 600px) { .crop-inputs { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
</style>
