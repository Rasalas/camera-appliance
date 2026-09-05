<template>
  <section class="panel card">
    <div class="panel-head"><h2>Einzelbild hochladen</h2><RouterLink class="btn sm ghost" to="/system/bild-upload">Server einstellen</RouterLink></div>
    <p class="mono-mute">Jeder Upload nimmt ein neues JPEG aus der Kamera auf. Der Ausschnitt bezieht sich auf das Originalbild, unabhängig von Drehung, Spiegelung und Zuschnitt der Kameras-Ansicht.</p>
    <div v-if="error" class="notice err" role="alert">{{ error }}</div>
    <div v-if="message" class="notice ok upload-message" role="status">{{ message }}</div>
    <div v-if="loading" class="empty">Upload-Einstellungen werden geladen…</div>
    <div v-else class="split-3-2">
      <div>
        <div v-if="imageSrc && !imageMissing" class="crop-stage" :class="{ selecting: crop.enabled && !working }" @pointerdown="startSelection" @pointermove="moveSelection" @pointerup="endSelection" @pointercancel="endSelection">
          <img :src="imageSrc" alt="Originalbild der Kamera zur Auswahl des Upload-Ausschnitts" draggable="false" @error="imageMissing = true" />
          <div v-if="crop.enabled && validCrop" class="crop-selection" :style="selectionStyle"><span>Upload-Ausschnitt</span></div>
        </div>
        <div v-else class="empty">Lade eine Vorschau, um den Ausschnitt im Kamerabild zu sehen.</div>
        <p class="mono-mute">{{ crop.enabled ? 'Ziehe im Bild einen Rahmen oder gib die Werte rechts in Prozent ein.' : 'Das vollständige Originalbild wird hochgeladen.' }} Die Vorschau dient zur Auswahl; das hochgeladene Bild wird frisch aufgenommen.</p>
      </div>
      <fieldset class="upload-controls" :disabled="working || cameraBusy">
        <label class="field"><span class="lbl">Bildbereich</span><select v-model="crop.enabled"><option :value="false">Gesamtes Bild</option><option :value="true">Nur Bildausschnitt</option></select></label>
        <div v-if="crop.enabled" class="crop-inputs">
          <label class="field"><span class="lbl">Links %</span><input v-model.number="crop.x" type="number" min="0" max="99" step="0.1" /></label>
          <label class="field"><span class="lbl">Oben %</span><input v-model.number="crop.y" type="number" min="0" max="99" step="0.1" /></label>
          <label class="field"><span class="lbl">Breite %</span><input v-model.number="crop.width" type="number" min="0.1" max="100" step="0.1" /></label>
          <label class="field"><span class="lbl">Höhe %</span><input v-model.number="crop.height" type="number" min="0.1" max="100" step="0.1" /></label>
        </div>
        <p v-if="!validCrop" class="notice err" role="alert">Der Ausschnitt muss vollständig im Bild liegen. Breite und Höhe müssen größer als 0 sein.</p>
        <div class="btn-row"><button class="btn" type="button" :disabled="!canCapture" @click="$emit('preview')">Vorschau aufnehmen</button><button class="btn" type="button" :disabled="!validCrop" @click="saveCrop">Bildbereich speichern</button></div>
        <div v-if="destination?.password_set" class="mono-mute">Ziel: {{ destination.protocol.toUpperCase() }} · {{ destination.host }} · {{ destination.directory }}</div>
        <p v-else class="mono-mute">Speichere zuerst einen Server mit Passwort unter <RouterLink to="/system/bild-upload">System → Bild-Upload</RouterLink>.</p>
        <button class="btn primary" type="button" :disabled="!validCrop || !canCapture || !destination?.password_set" @click="upload">{{ uploading ? 'Bild wird aufgenommen und hochgeladen…' : 'Jetzt aufnehmen & hochladen' }}</button>
      </fieldset>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../api/client'
import type { UploadCrop, UploadSettings } from '../types'

const props = defineProps<{ deviceId: string; imageSrc: string; username: string; password: string; stream: string; canCapture: boolean; cameraBusy: boolean }>()
defineEmits<{ preview: [] }>()
const crop = ref<UploadCrop>({ enabled: false, x: 0, y: 0, width: 100, height: 100 })
const destination = ref<UploadSettings>()
const loading = ref(true)
const working = ref(false)
const uploading = ref(false)
const error = ref('')
const message = ref('')
const imageMissing = ref(false)
let drag: { x: number; y: number; pointer: number } | null = null
const validCrop = computed(() => !crop.value.enabled || ([crop.value.x, crop.value.y, crop.value.width, crop.value.height].every(Number.isFinite) && crop.value.x >= 0 && crop.value.y >= 0 && crop.value.width > 0 && crop.value.height > 0 && crop.value.x + crop.value.width <= 100 && crop.value.y + crop.value.height <= 100))
const selectionStyle = computed(() => ({ left: `${crop.value.x}%`, top: `${crop.value.y}%`, width: `${crop.value.width}%`, height: `${crop.value.height}%` }))
watch(() => props.imageSrc, () => { imageMissing.value = false })

function point(event: PointerEvent) {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  return { x: Math.max(0, Math.min(100, (event.clientX - rect.left) / rect.width * 100)), y: Math.max(0, Math.min(100, (event.clientY - rect.top) / rect.height * 100)) }
}
function startSelection(event: PointerEvent) {
  if (!crop.value.enabled || working.value || props.cameraBusy || event.button !== 0) return
  event.preventDefault()
  drag = { ...point(event), pointer: event.pointerId }
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
  drag = null
  ;(event.currentTarget as HTMLElement).releasePointerCapture(event.pointerId)
}
async function saveCrop() {
  working.value = true
  error.value = ''; message.value = ''
  try { crop.value = await api.saveUploadCrop(props.deviceId, crop.value); message.value = 'Bildbereich gespeichert.' }
  catch (err) { error.value = err instanceof Error ? err.message : 'Bildbereich konnte nicht gespeichert werden.' }
  finally { working.value = false }
}
async function upload() {
  working.value = true; uploading.value = true
  error.value = ''; message.value = ''
  try {
    const result = await api.uploadSnapshot(props.deviceId, { username: props.username, password: props.password, stream: props.stream, crop: { ...crop.value } })
    message.value = `Hochgeladen: ${result.filename} · ${result.width} × ${result.height} Pixel · ${Math.ceil(result.bytes / 1024)} KB`
  } catch (err) { error.value = err instanceof Error ? err.message : 'Bild konnte nicht hochgeladen werden.' }
  finally { working.value = false; uploading.value = false }
}
onMounted(async () => {
  try { [crop.value, destination.value] = await Promise.all([api.uploadCrop(props.deviceId), api.uploadSettings()]) }
  catch (err) { error.value = err instanceof Error ? err.message : 'Upload-Einstellungen konnten nicht geladen werden.' }
  finally { loading.value = false }
})
</script>

<style scoped>
.upload-controls { border: 0; padding: 0; margin: 0; min-width: 0; display: grid; gap: 16px; align-content: start; }
.crop-inputs { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.crop-stage { position: relative; overflow: hidden; line-height: 0; user-select: none; }
.crop-stage.selecting { cursor: crosshair; touch-action: none; }
.crop-stage img { display: block; width: 100%; height: auto; }
.crop-selection { position: absolute; border: 2px solid var(--accent, #d97735); box-shadow: 0 0 0 2000px #0007; pointer-events: none; box-sizing: border-box; }
.crop-selection span { position: absolute; top: 4px; left: 4px; background: #171916; color: #fff; padding: 5px; line-height: 1.2; font-size: 10px; white-space: nowrap; }
.upload-message { overflow-wrap: anywhere; }
</style>
