<template>
  <section class="panel card">
    <div class="panel-head">
      <h2>Support-Bundle</h2>
      <div class="right">Status · Viewer · Netzwerk · Logs</div>
    </div>
    <div class="split">
      <div class="field">
        <span class="lbl">Diagnosepaket</span>
        <div class="btn-row"><button class="btn primary" :disabled="creatingSupportBundle" @click="onSupportBundle">{{ creatingSupportBundle ? 'Erstellt…' : 'Support-Bundle erstellen' }}</button></div>
      </div>
    </div>
    <div v-if="supportBundleResult" class="notice ok">
      <span class="tag">FERTIG</span>
      <div class="support-result">
        <div>{{ supportBundleResult.path }}</div>
        <div class="mono-mute">{{ supportBundleResult.files.length }} Dateien · Zugangsdaten maskiert</div>
        <div v-if="supportBundleResult.warning" class="mono-mute">{{ supportBundleResult.warning }}</div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useSystem } from '../../composables/useSystem'
const { supportBundleResult, error, createSupportBundle } = useSystem()
const creatingSupportBundle = ref(false)
async function onSupportBundle() { creatingSupportBundle.value=true;try { await createSupportBundle() } catch(err) { error.value=err instanceof Error?err.message:'Support-Bundle konnte nicht erstellt werden.' } finally { creatingSupportBundle.value=false } }
</script>
