<template>
  <form class="panel card settings-form" @submit.prevent="save">
    <div class="panel-head"><h2>{{ title }}</h2></div>
    <fieldset :disabled="saving || !settingsLoaded"><slot /></fieldset>
    <div v-if="saveError" class="notice err" role="alert">{{ saveError }}</div>
    <div class="form-actions">
      <span class="mono-mute" role="status">{{ !settingsLoaded ? 'Wird geladen…' : saving ? 'Wird gespeichert…' : saveError ? 'Nicht gespeichert' : dirty ? 'Ungespeicherte Änderungen' : 'Gespeichert' }}</span>
      <div class="btn-row">
        <button class="btn ghost" type="button" :disabled="!dirty || saving" @click="cancel">Abbrechen</button>
        <button class="btn primary" type="submit" :disabled="!dirty || saving || !settingsLoaded">{{ saving ? 'Speichert…' : title + ' speichern' }}</button>
      </div>
    </div>
  </form>
</template>
<script setup lang="ts">
import { computed, ref } from 'vue'
import { useSystem } from '../composables/useSystem'
const props=defineProps<{ title: string; settingKeys: string[] }>()
const {settingsLoaded,settingsDirty,discardSettings,saveSettings,error}=useSystem()
const saving=ref(false),saveError=ref('')
const dirty=computed(()=>settingsDirty(props.settingKeys))
async function save() {
  if(saving.value)return
  saving.value=true;saveError.value=''
  try { if(!await saveSettings(props.settingKeys))saveError.value=error.value }
  finally { saving.value=false }
}
function cancel() { discardSettings(props.settingKeys);saveError.value='' }
</script>
<style scoped>
fieldset { display:grid;gap:20px;border:0;padding:0;margin:0;min-width:0; }
fieldset :deep(.field) { align-content:start; }
.form-actions { display:flex;justify-content:space-between;align-items:center;gap:14px;flex-wrap:wrap;border-top:1px solid var(--hairline);padding-top:16px; }
.form-actions .mono-mute { font-size:11px; }
</style>
