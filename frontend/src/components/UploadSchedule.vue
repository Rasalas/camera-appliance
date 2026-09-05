<template>
  <div class="upload-schedule">
    <div class="schedule-row">
      <label class="interval-label"><span>Automatisch</span>
        <select :value="config.enabled ? config.interval_seconds : 0" :disabled="loading || saving" @change="changeInterval">
          <option :value="0">Aus</option><option :value="60">Jede Minute</option><option :value="300">Alle 5 Minuten</option><option :value="900">Alle 15 Minuten</option><option :value="3600">Jede Stunde</option>
          <option v-if="![60, 300, 900, 3600].includes(config.interval_seconds)" :value="config.interval_seconds">Alle {{ config.interval_seconds / 60 }} Minuten</option>
        </select>
      </label>
      <span class="mono-mute schedule-status" role="status">{{ saving ? 'Wird gespeichert…' : status?.running ? 'Upload läuft…' : status?.quiet_now ? `Ruhezeit bis ${config.quiet_hours.end}` : config.enabled && status?.next_run ? `Nächster Upload ${runTime(status.next_run)}` : 'Nur auf Knopfdruck' }}</span>
    </div>
    <div v-if="error || pollError || status?.last_error" class="notice err" role="alert">{{ error || pollError || `Letzter automatischer Upload: ${status?.last_error}` }}</div>
    <details v-if="config.enabled" class="advanced">
      <summary>{{ config.quiet_hours.enabled ? `Ruhezeit · ${config.quiet_hours.start} bis ${config.quiet_hours.end}` : 'Ruhezeit festlegen' }}</summary>
      <fieldset class="quiet-fields" :disabled="saving || loading">
        <label class="quiet-toggle"><input v-model="config.quiet_hours.enabled" type="checkbox" @change="save(config)" />Täglich pausieren</label>
        <template v-if="config.quiet_hours.enabled">
          <label>Von <input v-model="config.quiet_hours.start" type="time" required @change="save(config)" /></label>
          <label>Bis <input v-model="config.quiet_hours.end" type="time" required @change="save(config)" /></label>
        </template>
      </fieldset>
    </details>
    <div v-if="status && config.enabled" class="mono-mute time-note">Gerätezeit {{ clockTime(status.device_time) }} · {{ status.time_zone }}<span v-if="status.last_success"> · Zuletzt hochgeladen {{ runTime(status.last_success) }}</span></div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '../api/client'
import type { UploadScheduleInput, UploadScheduleStatus } from '../types'

const props = defineProps<{ deviceId: string; beforeEnable: () => Promise<boolean> }>()
const config = ref<UploadScheduleInput>({ enabled: false, interval_seconds: 60, quiet_hours: { enabled: false, start: '22:00', end: '07:00' } })
const status = ref<UploadScheduleStatus>()
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const pollError = ref('')
let generation = 0
let disposed = false
let timer: ReturnType<typeof setTimeout>
function clockTime(value: string) { return value.slice(11, 16) }
function runTime(value: string) { return value.slice(0, 10) === status.value?.device_time.slice(0, 10) ? clockTime(value) : `${value.slice(8, 10)}.${value.slice(5, 7)}. ${clockTime(value)}` }
function apply(result: UploadScheduleStatus) {
  status.value = result
  config.value = { enabled: result.enabled, interval_seconds: result.interval_seconds, quiet_hours: { ...result.quiet_hours } }
}
async function poll() {
  const started = generation
  try {
    const result = await api.uploadSchedule(props.deviceId)
    if (!disposed && started === generation && !saving.value) {
      if (loading.value) { apply(result); loading.value = false }
      else status.value = result
      pollError.value = ''
    }
  } catch (err) { if (!disposed && started === generation) pollError.value = err instanceof Error ? err.message : 'Zeitsteuerung nicht erreichbar.' }
  finally { if (!disposed) timer = setTimeout(() => void poll(), 5000) }
}
async function save(input: UploadScheduleInput) {
  if (saving.value) return
  generation++
  saving.value = true; error.value = ''
  const draft = { ...input, quiet_hours: { ...input.quiet_hours } }
  try {
    if (draft.enabled && !await props.beforeEnable()) throw new Error('Bitte zuerst Bildausschnitt, Privatbereiche, Zeitangabe und Dateieinstellungen prüfen und speichern lassen.')
    apply(await api.saveUploadSchedule(props.deviceId, draft))
  } catch (err) { error.value = err instanceof Error ? err.message : 'Zeitsteuerung konnte nicht gespeichert werden.' }
  finally { saving.value = false }
}
function changeInterval(event: Event) {
  const seconds = Number((event.target as HTMLSelectElement).value)
  void save({ ...config.value, enabled: seconds !== 0, interval_seconds: seconds || config.value.interval_seconds })
}
onMounted(() => void poll())
onBeforeUnmount(() => { disposed = true; clearTimeout(timer) })
</script>

<style scoped>
.upload-schedule { display: grid; gap: 12px; border-top: 1px solid var(--line); padding-top: 14px; }
.schedule-row, .interval-label, .quiet-fields, .quiet-fields label { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.schedule-row { justify-content: space-between; }
.interval-label { font-size: 12px; }
.interval-label select { width: auto; min-width: 165px; }
.schedule-status, .time-note { font-size: 11px; }
.quiet-fields { border: 0; padding: 12px 0 0; margin: 0; font-size: 12px; }
.quiet-fields input[type="time"] { width: 110px; padding: 8px 10px; border: 1px solid var(--line); border-radius: 6px; background: var(--surface); color: var(--ink); font: inherit; }
.quiet-fields input[type="checkbox"] { width: auto; accent-color: #c5ec68; }
</style>
