<template>
  <section class="panel card snapshot-editor">
    <div class="panel-head">
      <div><div class="eyebrow">{{ cameraLabel }}</div><h2>Bild-Upload</h2></div>
      <RouterLink class="server-link mono-mute" to="/system/bild-upload">Server einstellen</RouterLink>
    </div>
    <div class="crop-toolbar">
      <fieldset class="crop-mode" :disabled="cropLoading || uploading" aria-label="Bildbereich">
        <label :class="{ selected: !crop.enabled }"><input v-model="crop.enabled" type="radio" :value="false" name="upload-area" />Vollbild</label>
        <label :class="{ selected: crop.enabled }"><input v-model="crop.enabled" type="radio" :value="true" name="upload-area" />Ausschnitt</label>
      </fieldset>
      <span class="mono-mute save-status" role="status" aria-live="polite">{{ saveStatus }}</span>
    </div>
    <div v-if="settingsError || saveError" class="notice err" role="alert">{{ settingsError || saveError }}</div>
    <div v-if="previewSrc && !imageMissing" class="crop-stage" :class="{ selecting: crop.enabled && !uploading && !cropLoading }" @pointerdown="startSelection" @pointermove="moveSelection" @pointerup="endSelection" @pointercancel="cancelSelection">
      <img :src="previewSrc" :alt="`Kameravorschau ${cameraLabel}`" draggable="false" @error="imageMissing = true" />
      <div v-if="crop.enabled && validCrop" class="crop-selection" :style="selectionStyle"><span>Ausschnitt</span></div>
      <span v-if="previewLoading" class="preview-status">Vorschau lädt…</span>
    </div>
    <div v-else class="empty">{{ previewLoading ? 'Vorschau lädt…' : previewError || 'Keine Kameravorschau verfügbar.' }}</div>
    <div v-if="previewError" class="preview-error mono-mute" role="alert">{{ previewSrc && !imageMissing ? previewError : '' }} <button v-if="canCapture" class="retry-link" type="button" :disabled="previewLoading" @click="loadPreview">Erneut laden</button></div>
    <UploadNaming ref="naming" :device-id="deviceId" :busy="uploading" />
    <UploadSchedule :device-id="deviceId" :before-enable="prepareSchedule" />
    <div class="upload-footer">
      <span class="mono-mute">{{ destination?.password_set ? `${destination.protocol.toUpperCase()} · ${destination.host}` : 'Upload-Server einrichten' }}</span>
      <button class="btn primary" type="button" :disabled="cropLoading || previewLoading || uploading || cameraBusy || !validCrop || !canCapture || !destination?.password_set" @click="upload">{{ uploading ? 'Wird hochgeladen…' : 'Jetzt hochladen' }}</button>
    </div>
    <div v-if="uploadError" class="notice err" role="alert">{{ uploadError }}</div>
    <div v-if="uploadMessage" class="notice ok" role="status">{{ uploadMessage }}</div>
    <details v-if="crop.enabled" class="advanced">
      <summary>Genaue Werte</summary>
      <fieldset class="crop-inputs" :disabled="cropLoading || uploading">
        <label class="field"><span class="lbl">Links %</span><input v-model.number="crop.x" type="number" min="0" max="99" step="0.1" /></label>
        <label class="field"><span class="lbl">Oben %</span><input v-model.number="crop.y" type="number" min="0" max="99" step="0.1" /></label>
        <label class="field"><span class="lbl">Breite %</span><input v-model.number="crop.width" type="number" min="0.1" max="100" step="0.1" /></label>
        <label class="field"><span class="lbl">Höhe %</span><input v-model.number="crop.height" type="number" min="0.1" max="100" step="0.1" /></label>
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
import { createCropAutosave, validUploadCrop } from '../composables/uploadCropDraft'
import type { UploadCrop, UploadSettings } from '../types'

const props = defineProps<{ deviceId: string; cameraLabel: string; imageSrc: string; username: string; password: string; stream: string; canCapture: boolean; cameraBusy: boolean }>()
const crop = ref<UploadCrop>({ enabled: false, x: 0, y: 0, width: 100, height: 100 })
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
const imageMissing = ref(false)
let drag: { x: number; y: number; pointer: number; original: UploadCrop } | null = null
const validCrop = computed(() => validUploadCrop(crop.value))
const selectionStyle = computed(() => ({ left: `${crop.value.x}%`, top: `${crop.value.y}%`, width: `${crop.value.width}%`, height: `${crop.value.height}%` }))
const autosave = createCropAutosave(
  (draft) => api.saveUploadCrop(props.deviceId, draft),
  (state, err) => {
    saveStatus.value = { pending: 'Wird gespeichert…', saving: 'Wird gespeichert…', saved: 'Gespeichert', error: 'Nicht gespeichert' }[state]
    saveError.value = state === 'error' ? (err instanceof Error ? err.message : 'Ausschnitt konnte nicht gespeichert werden.') : ''
  }
)

watch(() => props.imageSrc, (src) => { previewSrc.value = src; imageMissing.value = false })
watch(crop, scheduleSave, { deep: true, flush: 'sync' })

function scheduleSave() {
  if (cropLoading.value || drag) return
  if (!validCrop.value) { autosave.cancelPending(); saveStatus.value = 'Nicht gespeichert'; return }
  autosave.change(crop.value)
}
async function prepareSchedule() {
  if (cropLoading.value || !validCrop.value || drag) return false
  await autosave.flush()
  return !saveError.value && !!await naming.value?.flush()
}
function point(event: PointerEvent) {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  return { x: Math.max(0, Math.min(100, (event.clientX - rect.left) / rect.width * 100)), y: Math.max(0, Math.min(100, (event.clientY - rect.top) / rect.height * 100)) }
}
function startSelection(event: PointerEvent) {
  if (!crop.value.enabled || uploading.value || cropLoading.value || event.button !== 0) return
  event.preventDefault()
  autosave.cancelPending()
  drag = { ...point(event), pointer: event.pointerId, original: { ...crop.value } }
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
}
function moveSelection(event: PointerEvent) {
  if (!drag || drag.pointer !== event.pointerId) return
  const p = point(event)
  const x = Math.floor(Math.min(drag.x, p.x) * 10) / 10
  const y = Math.floor(Math.min(drag.y, p.y) * 10) / 10
  crop.value = { enabled: true, x, y, width: Math.floor(Math.abs(drag.x - p.x) * 10) / 10, height: Math.floor(Math.abs(drag.y - p.y) * 10) / 10 }
}
function endSelection(event: PointerEvent) {
  if (!drag || drag.pointer !== event.pointerId) return
  if (!validCrop.value) crop.value = drag.original
  drag = null
  const stage = event.currentTarget as HTMLElement
  if (stage.hasPointerCapture(event.pointerId)) stage.releasePointerCapture(event.pointerId)
  scheduleSave()
}
function cancelSelection(event: PointerEvent) {
  if (!drag || drag.pointer !== event.pointerId) return
  crop.value = drag.original
  endSelection(event)
}
async function loadPreview() {
  if (!props.canCapture) { previewError.value = 'Bitte zuerst den Kamerazugang einrichten.'; return }
  previewLoading.value = true; previewError.value = ''
  try {
    const frame = await api.captureFrame(props.deviceId, { username: props.username, password: props.password, stream: props.stream })
    previewSrc.value = `data:${frame.content_type};base64,${frame.image_base64}`
    imageMissing.value = false
  } catch (err) { previewError.value = err instanceof Error ? err.message : 'Vorschau konnte nicht geladen werden.' }
  finally { previewLoading.value = false }
}
async function upload() {
  uploading.value = true; uploadError.value = ''; uploadMessage.value = ''
  try {
    if (!await naming.value?.flush()) throw new Error('Bitte zuerst einen gültigen Dateinamen speichern lassen.')
    const result = await api.uploadSnapshot(props.deviceId, { username: props.username, password: props.password, stream: props.stream, crop: { ...crop.value } })
    uploadMessage.value = `Hochgeladen · ${result.filename} · ${result.width} × ${result.height} Pixel`
  } catch (err) { uploadError.value = err instanceof Error ? err.message : 'Bild konnte nicht hochgeladen werden.' }
  finally { uploading.value = false }
}
onMounted(() => {
  void api.uploadCrop(props.deviceId).then((saved) => { crop.value = saved; cropLoading.value = false }).catch((err) => { settingsError.value = err instanceof Error ? err.message : 'Ausschnitt konnte nicht geladen werden.' })
  void api.uploadSettings().then((settings) => { destination.value = settings }).catch((err) => { settingsError.value = err instanceof Error ? err.message : 'Upload-Server konnte nicht geladen werden.' })
  void loadPreview()
})
onBeforeUnmount(() => { void autosave.close() })
</script>

<style scoped>
.snapshot-editor { gap: 16px; }
.notice { overflow-wrap: anywhere; }
.panel-head .eyebrow { margin-bottom: 6px; }
.server-link { font-size: 11px; }
.crop-toolbar, .upload-footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.crop-mode { display: flex; gap: 2px; border: 0; padding: 3px; margin: 0; background: var(--bg); border-radius: 6px; }
.crop-mode label { position: relative; padding: 8px 16px; cursor: pointer; border-radius: 4px; font-size: 12px; color: var(--ink-mute); }
.crop-mode label.selected { background: var(--ink); color: var(--bg); }
.crop-mode label:has(input:focus-visible) { outline: 2px solid var(--ink); outline-offset: 3px; }
.crop-mode input { position: absolute; opacity: 0; width: 1px; height: 1px; }
.save-status { font-size: 11px; }
.crop-inputs { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; border: 0; margin: 12px 0 0; padding: 0; }
.crop-stage { position: relative; overflow: hidden; line-height: 0; user-select: none; border-radius: 4px; }
.crop-stage.selecting { cursor: crosshair; touch-action: none; }
.crop-stage img { display: block; width: 100%; height: auto; }
.crop-selection { position: absolute; border: 2px solid #c5ec68; box-shadow: 0 0 0 2000px #0007; pointer-events: none; box-sizing: border-box; }
.crop-selection span, .preview-status { position: absolute; top: 6px; left: 6px; background: #171916; color: #fff; padding: 6px; line-height: 1.2; font-size: 10px; white-space: nowrap; }
.retry-link { border: 0; padding: 0; color: var(--ink); background: transparent; text-decoration: underline; cursor: pointer; }
.preview-error, .upload-footer > span { font-size: 11px; }
@media (max-width: 600px) { .crop-inputs { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
</style>
