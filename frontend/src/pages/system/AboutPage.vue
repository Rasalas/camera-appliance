<template>
  <section class="about-intro">
    <img src="/favicon.svg" alt="" width="64" height="64" />
    <p class="about-name">Watch<em>deck</em></p>
    <p>Kamerabilder im Blick. Einstellungen an einem Ort.</p>
    <p class="mono-mute">Watchdeck ist die Oberfläche von Camera Appliance, einem Projekt von {{ project.author }}. Die Station bündelt lokale Kamerastreams, verwaltet Zugänge und lädt bei Bedarf Einzelbilder auf deinen Server.</p>
  </section>
  <section class="panel card about-section"><div class="panel-head"><h2>Projekt</h2></div><dl class="spec"><div><dt>Entwicklung</dt><dd>{{ project.author }}</dd></div><div><dt>Version</dt><dd>{{ version || 'Wird geladen…' }}</dd></div></dl><p v-if="versionError" role="status" class="mono-mute">{{ versionError }}</p><a class="about-link" :href="project.repository" target="_blank" rel="noopener noreferrer"><AppIcon name="external" /><span><strong>GitHub</strong><small>Quellcode, Issues und Beiträge</small></span><span aria-hidden="true">↗</span></a></section>
  <div id="updates" class="about-updates"><UpdatesPage /></div>
  <section class="panel card about-section"><div class="panel-head"><h2>Unterstützen</h2></div><p class="mono-mute">Wenn dir Watchdeck hilft, kannst du die Weiterentwicklung unterstützen.</p><a class="about-link" :href="project.coffee" target="_blank" rel="noopener noreferrer"><AppIcon name="coffee" /><span><strong>Buy Me a Coffee</strong><small>Einmaliger Beitrag</small></span><span aria-hidden="true">↗</span></a><a class="about-link" :href="project.sponsors" target="_blank" rel="noopener noreferrer"><AppIcon name="heart" /><span><strong>GitHub Sponsors</strong><small>Regelmäßige Unterstützung</small></span><span aria-hidden="true">↗</span></a></section>
  <section class="panel card about-section"><div class="panel-head"><h2>Lizenz und Betrieb</h2></div><p>Camera Appliance steht unter der MIT-Lizenz.</p><p class="mono-mute">Icons von Lucide. <a href="/licenses/lucide.txt" target="_blank" rel="noopener noreferrer">Lizenzhinweise</a></p><details><summary>MIT-Lizenz lesen</summary><pre class="license-text">{{ license }}</pre></details><p class="mono-mute">Schriften werden von der Station ausgeliefert. Kamerastreams und Einstellungen verarbeitet die Station im lokalen Netzwerk. Externe Verbindungen entstehen etwa bei Update-Prüfungen und konfigurierten Bild-Uploads.</p><RouterLink class="about-link" to="/system/wartung/support"><AppIcon name="support" /><span><strong>Support</strong><small>Hilfe anfragen und Diagnosepaket erstellen</small></span><span aria-hidden="true">→</span></RouterLink></section>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../../api/client'
import { project } from '../../project'
import license from '../../../../LICENSE?raw'
import UpdatesPage from '../maintenance/UpdatesPage.vue'
import AppIcon from '../../components/AppIcon.vue'
const version = ref(''), versionError = ref('')
onMounted(async () => { try { const info = await api.health(); version.value = `${info.version} · ${info.commit}` } catch { version.value = 'Nicht verfügbar'; versionError.value = 'Die installierte Version konnte nicht abgefragt werden.' } })
</script>
<style scoped>
.about-intro { max-width:640px;display:grid;gap:14px; }
.about-intro p { margin:0;line-height:1.6; }
.about-intro .about-name { font:44px/1 var(--serif); }
.about-name em { color:var(--ink-soft); }
.about-updates { display:grid;gap:24px;max-width:760px;scroll-margin-top:24px; }
.about-section { display:grid;gap:16px;max-width:760px; }
.about-section p { margin:0;line-height:1.6; }
.about-link { display:flex;align-items:center;gap:16px;padding:16px 0;border-top:1px solid var(--hairline); }
.about-link > span:first-of-type { flex:1;display:grid;gap:6px; }
.about-link strong { font-weight:500; }
.about-link small { color:var(--ink-mute);font-size:13px; }
.about-link:hover { color:var(--live); }
summary { cursor:pointer;min-height:44px; }
.license-text { font-size:13px;white-space:pre-wrap;overflow-wrap:anywhere;line-height:1.6; }
</style>
