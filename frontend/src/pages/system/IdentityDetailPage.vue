<template>
  <RouterLink to="/system/identitaeten" class="mono-mute">← Identitäten</RouterLink>
  <div v-if="!settingsLoaded" role="status">Wird geladen…</div>
  <template v-else-if="identity">
    <EditableSection title="Kamera-Login" @edit="editing=true"><dl class="spec"><div><dt>Name</dt><dd>{{ identity.name }}</dd></div><div><dt>Benutzername</dt><dd>{{ identity.username }}</dd></div><div><dt>Passwort</dt><dd>{{ identity.password_set ? 'Gespeichert' : 'Nicht gesetzt' }}</dd></div></dl></EditableSection>
    <button class="btn danger" style="align-self:start" @click="deleting=true">Identität entfernen</button>
    <button class="mobile-fab" aria-label="Identität bearbeiten" @click="editing=true"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m4 16 12-12 4 4-12 12H4zM14 6l4 4"/></svg></button>
    <IdentityEditor :open="editing" :identity="identity" @close="editing=false" />
    <AdminDialog :open="deleting" title="Identität entfernen?" :busy="busy" compact @close="deleting=false"><p>„{{ identity.name }}“ wird entfernt. Kameras mit diesem gemeinsamen Login können anschließend einen neuen Zugang benötigen.</p><p v-if="error" role="alert">{{ error }}</p><div class="form-actions"><button class="btn" @click="deleting=false">Abbrechen</button><button class="btn danger" :disabled="busy" @click="remove">Entfernen</button></div></AdminDialog>
  </template>
  <p v-else>Diese Identität ist nicht mehr vorhanden.</p>
</template>
<script setup lang="ts">
import EditableSection from '../../components/EditableSection.vue'
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useSystem } from '../../composables/useSystem'
import IdentityEditor from '../../components/IdentityEditor.vue'
import AdminDialog from '../../components/AdminDialog.vue'
const route=useRoute(),router=useRouter()
const {credentialIdentities,settingsLoaded,loadAll,deleteCredentialIdentity}=useSystem()
const identity=computed(()=>credentialIdentities.value.find(item=>item.id===route.params.id))
const editing=ref(false),deleting=ref(false),busy=ref(false),error=ref('')
async function remove() { if(!identity.value)return;busy.value=true;try {await deleteCredentialIdentity(identity.value.id);deleting.value=false;await router.push('/system/identitaeten')} catch(err){error.value=err instanceof Error?err.message:'Entfernen fehlgeschlagen.'}finally{busy.value=false} }
onMounted(()=>void loadAll())
</script>
