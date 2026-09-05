<template>
  <section class="panel card">
    <div class="panel-head"><h2>Software aktualisieren</h2></div>
    <p class="mono-mute">{{ versionDetail }}</p>
    <UpdateInfo :phase="flow?.phase || 'idle'" :status="flow" />
    <div class="btn-row">
      <button v-if="flow?.phase === 'available'" class="btn primary" :disabled="busy" @click="client.download()">Update herunterladen</button>
      <button v-else-if="flow?.phase === 'ready'" class="btn primary" :disabled="busy" @click="client.install()">Jetzt aktualisieren</button>
      <button v-else class="btn primary" :disabled="busy || working" @click="client.check()">{{ working ? 'Update läuft…' : 'Nach Updates suchen' }}</button>
    </div>
    <p class="mono-mute">Ein laufendes Update wird auch beim Wechsel auf eine andere Seite weiter überwacht.</p>
  </section>
  <details class="panel card">
    <summary>Anderes Release installieren</summary>
    <form @submit.prevent="customInstall">
      <fieldset :disabled="busy || working" class="custom-update">
        <label class="field"><span class="lbl">Release-URL</span><input aria-label="Release-URL" v-model="url" type="url" placeholder="https://…/camera-appliance-latest.tar.gz" /><span class="mono-mute">Leer verwendet das neueste GitHub-Release.</span></label>
        <label class="field"><span class="lbl">SHA-256-Prüfsumme · optional</span><input aria-label="SHA-256-Prüfsumme · optional" v-model="digest" placeholder="sha256:…" /></label>
        <button class="btn" type="submit">Release installieren</button>
      </fieldset>
    </form>
  </details>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import UpdateInfo from '../../components/UpdateInfo.vue'
import { useUpdateFlow } from '../../composables/useUpdateFlow'
import { useSystem } from '../../composables/useSystem'
import { api } from '../../api/client'
const { status: flow, busy, client } = useUpdateFlow()
const { versionDetail } = useSystem()
const url=ref(''), digest=ref('')
const working=computed(()=>flow.value?.phase==='installing'||flow.value?.phase==='downloading')
function customInstall() { return client.install(()=>api.startUpdate(url.value.trim()||undefined,digest.value.trim()||undefined)) }
onMounted(()=>void client.refresh())
</script>
<style scoped>
.custom-update { display:grid;gap:16px;border:0;padding:16px 0 0;margin:0; }
.custom-update .btn { justify-self:start; }
</style>
