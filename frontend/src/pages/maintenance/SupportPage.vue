<template>
  <p class="mono-mute">Beschreibe das Problem und ergänze bei Bedarf Diagnosedaten. Die Anfrage geht an {{ project.supportEmail }}.</p>
  <section class="panel card support-request">
    <div class="panel-head"><h2>Hilfe anfragen</h2></div>
    <label class="field"><span class="lbl">Was funktioniert nicht?</span><textarea v-model="description" rows="5" maxlength="1500" placeholder="Was hast du erwartet? Was ist stattdessen passiert? Seit wann tritt das Problem auf?" /></label>
    <label class="toggle-row"><input v-model="includeDiagnostics" type="checkbox" :disabled="!diagnostics" /><span>Diagnoseauszug in die E-Mail aufnehmen</span></label>
    <details class="support-preview"><summary>Diagnoseauszug prüfen</summary><p class="mono-mute">Version und letzte Ereignisse. Zugangsdaten werden maskiert. Du kannst den Auszug vor dem Weitergeben kürzen.</p><p v-if="reportError" class="notice err" role="alert">{{ reportError }} <button class="btn ghost" @click="loadReport">Erneut laden</button></p><label v-else class="field"><span class="sr-only">Diagnoseauszug</span><textarea v-model="diagnostics" rows="8" maxlength="2000" :disabled="loading" /></label><RouterLink to="/system/wartung/ereignisse" class="text-link">Vollständiges Ereignisprotokoll öffnen</RouterLink></details>
    <div class="btn-row"><a class="btn primary" :href="mailURL"><AppIcon name="mail" />Hilfe anfragen</a><button class="btn ghost" @click="copyRequest">Anfrage kopieren</button></div>
    <p class="mono-mute">Öffnet einen Entwurf in deinem Mailprogramm. Ein heruntergeladenes Support-Bundle fügst du dort als Anhang hinzu.</p>
    <p v-if="copyMessage" role="status" class="mono-mute">{{ copyMessage }}</p>
  </section>
  <section class="panel card support-bundle">
    <div class="panel-head"><h2>Support-Bundle</h2></div>
    <p>Das Diagnosepaket enthält Systemstatus, Kamera-Verbindungen, Ereignisse und Einstellungen mit maskierten Zugangsdaten.</p>
    <p class="mono-mute">Es enthält Netzwerk- und Systeminformationen. Prüfe den Inhalt vor dem Weitergeben.</p>
    <div class="btn-row"><button class="btn" :disabled="creating" @click="createBundle"><AppIcon name="plus" />{{ creating ? 'Wird erstellt…' : 'Support-Bundle erstellen' }}</button><a v-if="bundleURL" class="btn primary" :href="bundleURL" :download="bundleName"><AppIcon name="download" />Bundle herunterladen</a></div>
    <p v-if="bundleURL" role="status" class="mono-mute">{{ bundleName }} · Bereit zum Herunterladen und Anhängen.</p>
    <p v-if="bundleError" class="notice err" role="alert">{{ bundleError }}</p>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '../../api/client'
import { project } from '../../project'
import { supportMailURL } from '../../composables/supportDraft'
import { useDraftGuard } from '../../composables/discardChanges'
import AppIcon from '../../components/AppIcon.vue'
const description = ref(''), diagnostics = ref(''), includeDiagnostics = ref(false), loading = ref(true), reportError = ref('')
const loadedDiagnostics = ref('')
const creating = ref(false), bundleURL = ref(''), bundleName = ref(''), bundleError = ref(''), copyMessage = ref('')
const mailURL = computed(() => supportMailURL(project.supportEmail, description.value, diagnostics.value, includeDiagnostics.value))
useDraftGuard(() => !!description.value.trim() || diagnostics.value !== loadedDiagnostics.value, () => { description.value = ''; diagnostics.value = loadedDiagnostics.value })
async function loadReport() {
  loading.value = true; reportError.value = ''
  try {
    const report = await api.supportReport()
    diagnostics.value = [`Watchdeck ${report.version.version} · ${report.version.commit}`, '', ...(report.events || []).map(event => `${event.created_at} [${event.level}] ${event.type}: ${event.message}`)].join('\n').slice(0, 2000)
    loadedDiagnostics.value = diagnostics.value
  } catch (err) { reportError.value = err instanceof Error ? err.message : 'Diagnose konnte nicht geladen werden.' }
  finally { loading.value = false }
}
async function createBundle() {
  if (creating.value) return
  creating.value = true; bundleError.value = ''
  try {
    const blob = await api.downloadSupportBundle()
    if (bundleURL.value) URL.revokeObjectURL(bundleURL.value)
    bundleURL.value = URL.createObjectURL(blob)
    bundleName.value = `watchdeck-support-${new Date().toISOString().replace(/[:.]/g, '-')}.tar.gz`
  } catch (err) { bundleError.value = err instanceof Error ? err.message : 'Support-Bundle konnte nicht erstellt werden.' }
  finally { creating.value = false }
}
async function copyRequest() {
  const body = new URL(mailURL.value).searchParams.get('body') || ''
  try { await navigator.clipboard.writeText(`An: ${project.supportEmail}\nBetreff: Watchdeck · Hilfe anfragen\n\n${body}`); copyMessage.value = 'Anfrage kopiert.' }
  catch { copyMessage.value = 'Kopieren ist hier nicht verfügbar. Markiere die Problembeschreibung und den Diagnoseauszug zum manuellen Kopieren.' }
}
onMounted(loadReport)
onBeforeUnmount(() => { if (bundleURL.value) URL.revokeObjectURL(bundleURL.value) })
</script>
<style scoped>
.support-request,.support-bundle { display:grid;gap:18px; }
p { margin:0; }
textarea { width:100%;resize:vertical;line-height:1.6; }
.support-preview { border-top:1px solid var(--hairline);padding-top:16px; }
.support-preview summary { cursor:pointer;min-height:44px; }
.support-preview[open] > :not(summary) { margin-bottom:14px; }
.text-link { text-decoration:underline;text-underline-offset:4px; }
</style>
