<template>
  <AdminDialog ref="dialog" :open="open" :title="identity ? 'Identität bearbeiten' : 'Identität hinzufügen'" :dirty="dirty" :busy="saving" @close="$emit('close')">
    <form @submit.prevent="save"><label class="field"><span class="lbl">Name · Pflichtfeld</span><input aria-label="Name · Pflichtfeld" v-model="draft.name" required autofocus /></label><label class="field"><span class="lbl">Benutzername · Pflichtfeld</span><input aria-label="Benutzername · Pflichtfeld" v-model="draft.username" required autocomplete="off" /></label><label class="field"><span class="lbl">Passwort</span><input aria-label="Passwort" v-model="draft.password" type="password" autocomplete="new-password" :placeholder="identity ? 'Leer behält das gespeicherte Passwort' : 'Kamera-Passwort'" /></label><p v-if="error" class="notice err" role="alert">{{ error }}</p><div class="form-actions"><button class="btn" type="button" @click="dialog?.requestClose()">Abbrechen</button><button class="btn primary" :disabled="saving" type="submit"><AppIcon v-if="!identity" name="plus" />{{ saving ? 'Speichert…' : 'Identität speichern' }}</button></div></form>
  </AdminDialog>
</template>
<script setup lang="ts">
import AppIcon from './AppIcon.vue'
import { computed, reactive, ref, watch } from 'vue'
import AdminDialog from './AdminDialog.vue'
import { useSystem } from '../composables/useSystem'
import { useDraftGuard } from '../composables/discardChanges'
import type { CredentialIdentity } from '../types'
const props = defineProps<{open:boolean;identity?:CredentialIdentity}>()
const emit = defineEmits<{close:[];saved:[]}>()
const { saveCredentialIdentity } = useSystem()
const draft = reactive({id:'',name:'',username:'',password:''}), baseline = ref(''), error = ref(''), saving = ref(false)
const dialog = ref<InstanceType<typeof AdminDialog>>()
const dirty = computed(() => props.open && JSON.stringify(draft) !== baseline.value)
watch(() => props.open, open => { if (open) { Object.assign(draft,{id:props.identity?.id || '',name:props.identity?.name || '',username:props.identity?.username || '',password:''});baseline.value=JSON.stringify(draft);error.value='' } },{immediate:true})
useDraftGuard(() => dirty.value, () => emit('close'))
async function save() {
  if(saving.value)return
  saving.value=true;error.value=''
  try { await saveCredentialIdentity(draft);baseline.value=JSON.stringify(draft);emit('saved');emit('close') }
  catch(err) { error.value=err instanceof Error?err.message:'Identität konnte nicht gespeichert werden.' }
  finally { saving.value=false }
}
</script>
