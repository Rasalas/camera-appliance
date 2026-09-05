<template>
  <section class="panel edit-section">
    <div class="panel-head"><h2>{{ title }}</h2><button class="btn ghost" :disabled="!settingsLoaded" :aria-label="title + ' bearbeiten'" @click="editing = true">Bearbeiten</button></div>
    <div v-if="!settingsLoaded" role="status">Wird geladen…</div>
    <slot v-else name="summary"><p class="mono-mute">{{ title }} ist eingerichtet.</p></slot>
    <AdminDialog :open="editing" :title="title + ' bearbeiten'" :dirty="dirty" :busy="saving" @close="cancel">
      <form @submit.prevent="save">
        <fieldset :disabled="saving || !settingsLoaded"><slot /></fieldset>
        <div v-if="saveError" class="notice err" role="alert">{{ saveError }}</div>
        <div class="form-actions"><span role="status">{{ saving ? 'Wird gespeichert…' : saveError ? 'Nicht gespeichert' : dirty ? 'Ungespeicherte Änderungen' : 'Keine Änderungen' }}</span><button class="btn ghost" type="button" :disabled="saving" @click="requestCancel">Abbrechen</button><button class="btn primary" type="submit" :disabled="!dirty || saving">{{ title }} speichern</button></div>
      </form>
    </AdminDialog>
  </section>
</template>
<script setup lang="ts">
import { computed, ref } from 'vue'
import { useSystem } from '../composables/useSystem'
import { askDiscard, useDraftGuard } from '../composables/discardChanges'
import AdminDialog from './AdminDialog.vue'
const props = defineProps<{ title: string; settingKeys: string[]; afterSave?: () => Promise<void> }>()
const { settingsLoaded, settingsDirty, discardSettings, saveSettings, error } = useSystem()
const saving = ref(false), saveError = ref(''), editing = ref(false)
const dirty = computed(() => settingsDirty(props.settingKeys))
useDraftGuard(() => editing.value && dirty.value, cancel)
async function save() {
  if (saving.value) return
  saving.value = true; saveError.value = ''
  try {
    if (!await saveSettings(props.settingKeys)) { saveError.value = error.value; return }
    await props.afterSave?.()
    editing.value = false
  } catch (err) { saveError.value = err instanceof Error ? err.message : 'Änderung konnte nicht angewendet werden.' }
  finally { saving.value = false }
}
function cancel() { discardSettings(props.settingKeys); saveError.value = ''; editing.value = false }
async function requestCancel() { if (!dirty.value || await askDiscard()) cancel() }
defineExpose({ edit: () => { editing.value = true } })
</script>
<style scoped>
fieldset { display:grid;gap:20px;border:0;padding:0;margin:0;min-width:0; }
fieldset :deep(.field) { align-content:start; }
</style>
