<template>
  <div class="image-editor">
    <div class="image-tools">
      <fieldset class="crop-mode" :disabled="disabled" aria-label="Bildbereich">
        <label :class="{ selected: !crop.enabled }"><input type="radio" name="upload-area" :checked="!crop.enabled" @click="chooseCrop(false)" />Vollbild</label>
        <label :class="{ selected: crop.enabled }"><input type="radio" name="upload-area" :checked="crop.enabled" @click="chooseCrop(true)" />Ausschnitt</label>
      </fieldset>
      <div class="mask-tools">
        <button class="btn" :class="{ armed: tool === 'black' }" :disabled="disabled || config.masks.length >= 16" @click="arm('black')">+ Schwärzen</button>
        <button class="btn" :class="{ armed: tool === 'pixelate' }" :disabled="disabled || config.masks.length >= 16" @click="arm('pixelate')">+ Verpixeln</button>
      </div>
    </div>
    <div class="btn-row"><span class="mono-mute">Privatbereiche und Zeitangabe werden automatisch gespeichert.</span><button class="btn ghost" :disabled="disabled || !imageChanged" @click="restoreImage">Änderungen seit Öffnen zurücknehmen</button></div>
    <div class="image-help"><span>{{ tool === 'black' || tool === 'pixelate' ? 'Bereich im Bild aufziehen. Escape bricht ab.' : tool === 'crop' && crop.enabled ? 'Bildausschnitt aufziehen. Privatbereiche bleiben am Originalbild verankert.' : 'Privatbereiche auswählen, verschieben oder an der Ecke vergrößern.' }}</span><span role="status">{{ statusText }}</span></div>
    <div class="crop-stage" :class="{ selecting: tool !== 'edit' }" @pointerdown="start" @pointermove="move" @pointerup="finish" @pointercancel="cancel" @keydown.esc="cancel" tabindex="0" aria-label="Bildeditor">
      <canvas ref="canvas" :class="{ hidden: !sourceReady || !hasRendered || loading }" role="img" :aria-label="`Kameravorschau ${cameraLabel} mit Privatbereichen`" />
      <div v-if="!sourceReady || loading || !hasRendered" class="empty">{{ previewError || 'Vorschau lädt…' }}</div>
      <template v-else>
        <div v-if="crop.enabled && validUploadCrop(crop)" class="crop-selection" :style="style(crop)"><span>Ausschnitt</span></div>
        <div v-for="(mask,index) in config.masks" :key="mask.id" class="mask-outline" :class="{ active: mask.id === selectedId, editable: tool === 'edit' && !disabled }" :style="style(mask)" :data-mask-id="mask.id">
          <span>{{ index + 1 }} · {{ mask.mode === 'black' ? 'Schwarz' : 'Verpixelt' }}</span>
          <button v-if="selectedId === mask.id && tool === 'edit'" class="resize-handle" type="button" data-resize="true" aria-label="Bereich an der Ecke vergrößern" :disabled="disabled" />
        </div>
        <span v-if="previewLoading" class="preview-status">Vorschau lädt…</span>
      </template>
    </div>
    <div v-if="renderError" class="notice err" role="alert">{{ renderError }}</div>
    <div v-if="error" class="notice err" role="alert">{{ error }}<button v-if="loading" class="btn" @click="loadSettings">Erneut laden</button></div>
    <div v-if="config.masks.length" class="mask-list" aria-label="Privatbereiche">
      <button v-for="(mask,index) in config.masks" :key="mask.id" type="button" :aria-pressed="selectedId === mask.id" :disabled="disabled" @click="select(mask.id)">{{ index + 1 }} · {{ mask.mode === 'black' ? 'Schwarz' : 'Verpixelt' }}</button>
    </div>
    <fieldset v-if="selected" class="mask-edit" :disabled="disabled">
      <label>Bereich {{ config.masks.indexOf(selected) + 1 }} <select v-model="selected.mode" aria-label="Darstellung des Privatbereichs"><option value="black">Schwarz</option><option value="pixelate">Stark verpixelt</option></select></label>
      <button class="btn" type="button" @click="removeSelected">Entfernen</button>
      <details class="advanced"><summary>Genaue Position</summary><div class="mask-values">
        <label v-for="field in fields" :key="field.key" class="field"><span class="lbl">{{ field.label }} %</span><input aria-label="{{ field.label }} %" v-model.number="selected[field.key]" type="number" min="0" max="100" step="0.1" /></label>
      </div></details>
    </fieldset>
    <label class="timestamp-option"><input v-model="config.timestamp" type="checkbox" :disabled="disabled" />Datum und Uhrzeit einblenden <span class="mono-mute">Aufnahmezeit des Geräts · unten rechts</span></label>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api } from '../api/client'
import { createDraftAutosave } from '../composables/draftAutosave'
import { cloneImageSettings, paintImagePreview, validImageSettings } from '../composables/uploadImagePreview'
import { validUploadCrop } from '../composables/uploadCropDraft'
import type { UploadCrop, UploadImageSettings, UploadMask } from '../types'

const props = defineProps<{ deviceId: string; cameraLabel: string; src: string; capturedAt: string; crop: UploadCrop; busy: boolean; cropLoading: boolean; cropStatus: string; previewLoading: boolean; previewError: string }>()
const emit = defineEmits<{ 'update:crop': [crop: UploadCrop]; selecting: [active: boolean] }>()
const config = ref<UploadImageSettings>({ masks: [], timestamp: false })
const originalConfig = ref<UploadImageSettings>()
const imageChanged = computed(() => !!originalConfig.value && JSON.stringify(config.value) !== JSON.stringify(originalConfig.value))
function restoreImage() { if (originalConfig.value) { config.value = cloneImageSettings(originalConfig.value); selectedId.value = ''; void autosave.flush() } }
const loading = ref(true), error = ref(''), renderError = ref(''), imageStatus = ref('')
const canvas = ref<HTMLCanvasElement>(), sourceReady = ref(false), hasRendered = ref(false)
const selectedId = ref(''), tool = ref<'crop'|'edit'|'black'|'pixelate'>('crop')
const selected = computed(() => config.value.masks.find(m => m.id === selectedId.value))
const statusText = computed(() => [props.cropStatus,imageStatus.value].find(value => value && value !== 'Gespeichert') || imageStatus.value || props.cropStatus)
const disabled = computed(() => props.busy || props.cropLoading || loading.value || drawing.value)
const drawing = ref(false)
const fields = [{key:'x',label:'Links'},{key:'y',label:'Oben'},{key:'width',label:'Breite'},{key:'height',label:'Höhe'}] as const
let source: HTMLImageElement | undefined, scratch: HTMLCanvasElement | undefined, generation = 0, animation = 0, disposed = false
type Point = { x: number; y: number }
let drag: { kind: 'crop'|'new'|'move'|'resize'; start: Point; original: UploadCrop; settings: UploadImageSettings; pointer: number; element: HTMLElement; mode: UploadMask['mode'] } | undefined
let pendingMask: UploadMask | undefined
const autosave = createDraftAutosave((draft: UploadImageSettings) => api.saveUploadImageSettings(props.deviceId,draft), (state,err) => {
  imageStatus.value = {pending:'Wird gespeichert…',saving:'Wird gespeichert…',saved:'Gespeichert',error:'Nicht gespeichert'}[state]
  error.value = state === 'error' ? (err instanceof Error ? err.message : 'Privatbereiche konnten nicht gespeichert werden.') : ''
}, cloneImageSettings, validImageSettings)

const style = (r: Pick<UploadCrop,'x'|'y'|'width'|'height'>) => ({left:`${r.x}%`,top:`${r.y}%`,width:`${r.width}%`,height:`${r.height}%`})
function paint() {
  if (!source || !canvas.value || loading.value || disposed) return
  try {
    const draft = cloneImageSettings(config.value)
    if (pendingMask && validImageSettings({masks:[pendingMask],timestamp:false})) draft.masks.push(pendingMask)
    // Only publish a fully rendered preview. Failed edits keep the previous
    // protected frame and cannot flash a partially processed source image.
    scratch ||= document.createElement('canvas')
    paintImagePreview(scratch,source,draft,props.crop,props.capturedAt)
    canvas.value.width=scratch.width;canvas.value.height=scratch.height
    const ctx=canvas.value.getContext('2d')
    if(!ctx)throw new Error('Bildvorschau kann nicht bearbeitet werden. Upload gesperrt.')
    ctx.drawImage(scratch,0,0);hasRendered.value=true
    renderError.value = ''
  } catch (err) { if(drag?.kind!=='crop')renderError.value = err instanceof Error ? err.message : 'Maskierte Vorschau fehlgeschlagen. Upload gesperrt.' }
}
function requestPaint() { cancelAnimationFrame(animation); animation = requestAnimationFrame(paint) }
function changed() {
  requestPaint()
  if (loading.value || drag) return
  if (!validImageSettings(config.value)) { autosave.cancelPending(); error.value='Privatbereiche müssen im Bild liegen und größer als 0 sein.'; imageStatus.value='Nicht gespeichert'; return }
  autosave.change(config.value)
}
watch(config,changed,{deep:true,flush:'sync'})
watch(() => [props.crop,props.capturedAt],requestPaint,{deep:true})
watch(() => props.src,(src) => {
  const current=++generation
  sourceReady.value=false;hasRendered.value=false; source=undefined
  if (!src) return
  const img=new Image()
  img.onload=()=>{if(current===generation && !disposed){source=img;sourceReady.value=true;requestPaint()}}
  img.onerror=()=>{if(current===generation && !disposed)renderError.value='Vorschaubild konnte nicht geladen werden. Upload gesperrt.'}
  img.src=src
},{immediate:true})

function chooseCrop(enabled: boolean) { tool.value='crop';selectedId.value='';emit('update:crop',{...props.crop,enabled}) }
function arm(mode: UploadMask['mode']) { tool.value=mode;selectedId.value='' }
function select(id: string) { selectedId.value=id;tool.value='edit' }
function removeSelected() { config.value.masks=config.value.masks.filter(m=>m.id!==selectedId.value);selectedId.value='' }
function point(event: PointerEvent): Point {
  const b=(event.currentTarget as HTMLElement).getBoundingClientRect()
  return {x:Math.max(0,Math.min(100,(event.clientX-b.left)/b.width*100)),y:Math.max(0,Math.min(100,(event.clientY-b.top)/b.height*100))}
}
function start(event: PointerEvent) {
  if (disabled.value || !sourceReady.value || !hasRendered.value || event.button!==0) return
  const target=(event.target as HTMLElement).closest<HTMLElement>('[data-mask-id]')
  let kind: NonNullable<typeof drag>['kind']='crop'
  let original={...props.crop}
  if(tool.value==='black'||tool.value==='pixelate')kind='new'
  else if(tool.value==='edit') {
    const mask=config.value.masks.find(m=>m.id===target?.dataset.maskId)
    if(!mask)return
    selectedId.value=mask.id;original={...mask,enabled:true}
    kind=(event.target as HTMLElement).hasAttribute('data-resize')?'resize':'move'
  } else if(!props.crop.enabled)return
  event.preventDefault();autosave.cancelPending()
  const element=event.currentTarget as HTMLElement
  drag={kind,start:point(event),original,settings:cloneImageSettings(config.value),pointer:event.pointerId,element,mode:tool.value==='pixelate'?'pixelate':'black'}
  drawing.value=true;emit('selecting',true);element.focus({preventScroll:true});element.setPointerCapture(event.pointerId)
}
function move(event: PointerEvent) {
  if(!drag || drag.pointer!==event.pointerId)return
  const p=point(event), round=(v:number)=>Math.floor(v*10)/10
  let rect={x:round(Math.min(drag.start.x,p.x)),y:round(Math.min(drag.start.y,p.y)),width:round(Math.abs(p.x-drag.start.x)),height:round(Math.abs(p.y-drag.start.y))}
  if(drag.kind==='move')rect={width:drag.original.width,height:drag.original.height,x:round(Math.max(0,Math.min(100-drag.original.width,drag.original.x+p.x-drag.start.x))),y:round(Math.max(0,Math.min(100-drag.original.height,drag.original.y+p.y-drag.start.y)))}
  if(drag.kind==='resize')rect={x:drag.original.x,y:drag.original.y,width:round(Math.max(0.1,Math.min(100-drag.original.x,p.x-drag.original.x))),height:round(Math.max(0.1,Math.min(100-drag.original.y,p.y-drag.original.y)))}
  if(drag.kind==='crop')emit('update:crop',{...rect,enabled:true})
  else if(drag.kind==='new')pendingMask={id:'pending',mode:drag.mode,...rect}
  else if(selected.value)Object.assign(selected.value,rect)
  requestPaint()
}
function finish(event?: PointerEvent) {
  if(!drag || (event && event.pointerId!==drag.pointer))return
  if(drag.kind==='crop' && !validUploadCrop(props.crop))emit('update:crop',drag.original)
  if(drag.kind==='new' && pendingMask && validImageSettings({masks:[pendingMask],timestamp:false})) {
    const mask={...pendingMask,id:`mask-${Date.now().toString(36)}-${Math.random().toString(36).slice(2,10)}`};config.value.masks.push(mask);select(mask.id)
  }
  const active=drag;drag=undefined;pendingMask=undefined;drawing.value=false
  if(active.element.hasPointerCapture(active.pointer))active.element.releasePointerCapture(active.pointer)
  emit('selecting',false)
  if(active.kind!=='crop')changed()
  requestPaint()
}
function cancel() {
  if(drag){emit('update:crop',drag.kind==='crop'?drag.original:props.crop);config.value=drag.settings;pendingMask=undefined;finish()}
  tool.value='edit'
}
async function flush() {
  if(loading.value || drag || !sourceReady.value || !validImageSettings(config.value))return false
  paint()
  if(renderError.value)return false
  await autosave.flush()
  return !error.value
}
async function loadSettings() {
  try {
    const result=await api.uploadImageSettings(props.deviceId)
    if(!validImageSettings(result))throw new Error('Privatbereiche konnten nicht sicher geladen werden. Upload gesperrt.')
    if(disposed)return
    config.value=result;originalConfig.value=cloneImageSettings(result);loading.value=false;error.value='';requestPaint()
  } catch(err){error.value=err instanceof Error?err.message:'Privatbereiche konnten nicht geladen werden. Upload gesperrt.'}
}
onMounted(()=>void loadSettings())
onBeforeUnmount(()=>{if(drag)cancel();disposed=true;generation++;cancelAnimationFrame(animation);void autosave.close()})
defineExpose({flush})
</script>

<style scoped>
.image-editor { display:grid; gap:12px; }
.image-tools,.mask-tools,.image-help,.mask-list,.mask-edit,.mask-edit label,.timestamp-option { display:flex;align-items:center;gap:10px;flex-wrap:wrap; }
.image-tools,.image-help { justify-content:space-between; }
.image-help { font-size:10px;line-height:1.5;color:var(--ink-mute); }
.image-help [role=status] { margin-left:auto; }
.crop-mode { display:flex;gap:2px;border:0;padding:3px;margin:0;background:var(--bg);border-radius:6px; }
.crop-mode label { position:relative;padding:8px 16px;cursor:pointer;border-radius:4px;font-size:12px;color:var(--ink-mute); }
.crop-mode label.selected { background:var(--ink);color:var(--bg); }
.crop-mode label:has(input:focus-visible) { outline:2px solid var(--ink);outline-offset:3px; }
.crop-mode input { position:absolute;opacity:0;width:1px;height:1px; }
.mask-tools .btn { font-size:10px;padding:10px 12px; }
.mask-tools .armed { background:#c5ec68;color:#171916; }
.crop-stage { position:relative;overflow:hidden;line-height:0;user-select:none;touch-action:none;border-radius:4px; }
.crop-stage.selecting { cursor:crosshair; }
canvas { display:block;width:100%;height:auto; }
canvas.hidden { display:none; }
.crop-selection { position:absolute;border:2px solid #c5ec68;box-shadow:0 0 0 2000px #0007;pointer-events:none;box-sizing:border-box; }
.crop-selection span,.mask-outline > span,.preview-status { position:absolute;top:6px;left:6px;background:#171916;color:#fff;padding:6px;line-height:1.2;font-size:10px;white-space:nowrap;pointer-events:none; }
.mask-outline { position:absolute;border:1px dashed #ddd;box-sizing:border-box;pointer-events:none; }
.mask-outline.active { border:2px solid #c5ec68; }
.mask-outline.editable { pointer-events:auto;cursor:move; }
.mask-outline > span { top:0;left:0;font-size:9px;padding:4px; }
.resize-handle { position:absolute;bottom:0;right:0;width:18px;height:18px;border:2px solid #171916;background:#c5ec68;cursor:nwse-resize; }
.mask-list button { padding:7px 10px;background:var(--surface);border:1px solid transparent;border-radius:5px;color:var(--ink-mute);font:inherit;font-size:10px;cursor:pointer; }
.mask-list button[aria-pressed=true] { border-color:#c5ec68;color:var(--ink); }
.mask-edit { border:0;padding:0;margin:0;font-size:11px; }
.mask-edit select { width:auto;padding:8px; }
.mask-edit .btn { padding:8px 10px;font-size:10px; }
.mask-values { display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px;padding-top:12px; }
.timestamp-option { font-size:12px; }
.timestamp-option input { width:auto;accent-color:#c5ec68; }
.timestamp-option span { font-size:10px; }
.notice { overflow-wrap:anywhere; }
@media(max-width:600px){.mask-values { grid-template-columns:repeat(2,minmax(0,1fr)); }.timestamp-option span { width:100%;padding-left:24px; }}
</style>
