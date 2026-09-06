<template>
  <section id="ereignisse" class="panel card support-section">
    <div class="panel-head"><h2>Ereignisprotokoll</h2></div>
    <p v-if="loading" role="status" class="mono-mute">Protokoll wird geladen…</p>
    <div v-else-if="reportError" class="notice err" role="alert">{{ reportError }}<button class="btn ghost" @click="loadReport">Erneut laden</button></div>
    <p v-else-if="!events.length" class="mono-mute">Noch keine Ereignisse vorhanden.</p>
    <ol v-else class="support-events" aria-label="Letzte Ereignisse">
      <li v-for="event in preview" :key="event.id"><time :datetime="event.created_at">{{ formatTime(event.created_at) }}</time><span class="event-level">{{ event.level }}</span><span>{{ event.message }}</span></li>
    </ol>
    <p class="mono-mute">Die Vorschau zeigt bis zu fünf Einträge. Der Textdownload enthält bis zu 100 Ereignisse.</p>
    <div class="support-downloads">
      <button class="btn" :disabled="loading || !events.length" @click="downloadLog"><AppIcon name="download" />Protokoll herunterladen</button>
      <button class="btn" :disabled="creating" @click="downloadBundle"><AppIcon name="download" />{{ creating ? 'Diagnosepaket wird erstellt…' : 'Diagnosepaket herunterladen' }}</button>
    </div>
    <p class="mono-mute">Das Diagnosepaket ergänzt das Protokoll um Systemstatus, Kamera-Verbindungen und Einstellungen. Zugangsdaten sind maskiert; Netzwerkdaten bleiben enthalten.</p>
    <p v-if="downloadMessage" role="status" class="mono-mute">{{ downloadMessage }}</p>
    <p v-if="bundleError" class="notice err" role="alert">{{ bundleError }}</p>
  </section>
  <section class="panel card support-section">
    <div class="panel-head"><h2>Hilfe anfragen</h2></div>
    <label class="field"><span class="lbl">Was funktioniert nicht?</span><textarea v-model="description" rows="3" maxlength="1500" placeholder="Was ist passiert und seit wann tritt das Problem auf?" /></label>
    <label class="toggle-row"><input v-model="includeDiagnostics" type="checkbox" :disabled="!report" /><span>Version und Protokollvorschau in die E-Mail aufnehmen</span></label>
    <div class="support-actions"><a class="btn primary" :href="mailURL"><AppIcon name="mail" />Hilfe anfragen</a></div>
    <p class="mono-mute">Öffnet einen E-Mail-Entwurf an {{ project.supportEmail }}. Heruntergeladene Dateien fügst du bei Bedarf im Mailprogramm als Anhang hinzu.</p>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '../../api/client'
import { project } from '../../project'
import { supportMailURL } from '../../composables/supportDraft'
import { useDraftGuard } from '../../composables/discardChanges'
import AppIcon from '../../components/AppIcon.vue'
const description = ref(''), includeDiagnostics = ref(false), loading = ref(true), reportError = ref('')
const report = ref<Awaited<ReturnType<typeof api.supportReport>>>()
const events = computed(() => report.value?.events || [])
const preview = computed(() => events.value.slice(0, 5))
const creating = ref(false), bundleError = ref(''), downloadMessage = ref('')
let downloadURL = ''
const eventLines = (items: typeof events.value) => items.map(event => `${event.created_at} [${event.level}] ${event.type}: ${event.message}`).join('\n')
const diagnostics = computed(() => report.value ? `Watchdeck ${report.value.version.version} · ${report.value.version.commit}\n\n${eventLines(preview.value)}` : '')
const mailURL = computed(() => supportMailURL(project.supportEmail, description.value, diagnostics.value, includeDiagnostics.value))
useDraftGuard(() => !!description.value.trim(), () => { description.value = '' })
function formatTime(value: string) { return new Date(value).toLocaleString('de-DE', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' }) }
async function loadReport() {
  loading.value = true; reportError.value = ''
  try { report.value = await api.supportReport() }
  catch (err) { reportError.value = err instanceof Error ? err.message : 'Protokoll konnte nicht geladen werden.' }
  finally { loading.value = false }
}
function download(blob: Blob, name: string) {
  if (downloadURL) URL.revokeObjectURL(downloadURL)
  downloadURL = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = downloadURL; link.download = name
  document.body.append(link); link.click(); link.remove()
  downloadMessage.value = 'Download gestartet.'
}
function downloadLog() {
  download(new Blob([eventLines(events.value) + '\n'], { type: 'text/plain;charset=utf-8' }), 'watchdeck-ereignisprotokoll.txt')
}
async function downloadBundle() {
  if (creating.value) return
  creating.value = true; bundleError.value = ''; downloadMessage.value = ''
  try { download(await api.downloadSupportBundle(), `watchdeck-diagnose-${new Date().toISOString().replace(/[:.]/g, '-')}.tar.gz`) }
  catch (err) { bundleError.value = err instanceof Error ? err.message : 'Diagnosepaket konnte nicht erstellt werden.' }
  finally { creating.value = false }
}
onMounted(loadReport)
onBeforeUnmount(() => { if (downloadURL) URL.revokeObjectURL(downloadURL) })
</script>
<style scoped>
.support-section { display:grid;gap:18px;scroll-margin-top:24px; }
p { margin:0; }
textarea { width:100%;resize:vertical;line-height:1.6; }
.support-events { list-style:none;margin:0;padding:0; }
.support-events li { display:grid;grid-template-columns:110px 50px minmax(0,1fr);gap:12px;padding:12px 0;border-bottom:1px solid var(--hairline);font-size:13px;line-height:1.6;overflow-wrap:anywhere; }
.support-events time,.event-level { color:var(--ink-mute); }
.support-downloads,.support-actions { justify-content:flex-end;display:flex;gap:12px;flex-wrap:wrap; }
.support-actions { justify-content:flex-end; }
@media(max-width:820px) {
  .support-events li { grid-template-columns:1fr auto;gap:4px 12px; }
  .support-events li > span:last-child { grid-column:1/-1;font-size:14px; }
  .support-downloads { justify-content:flex-end; }
}
</style>
