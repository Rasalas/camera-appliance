<template>
  <section class="panel">
    <div class="panel-head"><h2>Kamera-Identitäten</h2><button class="btn primary desktop-primary" @click="editing=true">Identität hinzufügen</button></div>
    <p class="mono-mute">Wiederverwendbare Kamera-Logins. Beim Bildtest werden sie für Kameras ohne eigenes Passwort ausprobiert.</p>
    <p class="mono-mute">{{ credentialIdentities.length }} gespeichert · nach Name sortiert</p>
    <div v-if="!credentialIdentities.length" class="empty">Noch keine Identitäten. Füge einen gemeinsamen Kamera-Login hinzu.</div>
    <div v-else class="identity-list"><div v-for="identity in sorted" :key="identity.id" class="identity-resource-row" @click="openRow($event,identity.id)"><RouterLink :to="`/system/identitaeten/${identity.id}`">{{ identity.name }}</RouterLink><span>{{ identity.username }}</span><span class="mono-mute">{{ identity.password_set ? 'Passwort gespeichert' : 'Kein Passwort' }}</span><button class="btn ghost" :disabled="busy" @click="duplicate(identity)">Duplizieren</button></div></div>
  </section>
  <button class="mobile-fab" aria-label="Identität hinzufügen" @click="editing=true"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 4v16M4 12h16"/></svg></button>
  <IdentityEditor :open="editing" @close="editing=false" />
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useSystem } from '../../composables/useSystem'
import { rowDestination } from '../../composables/resourceRow'
import IdentityEditor from '../../components/IdentityEditor.vue'
import type { CredentialIdentity } from '../../types'
const { credentialIdentities, loadAll, saveCredentialIdentity, error } = useSystem()
const router=useRouter(), editing=ref(false), busy=ref(false)
const sorted=computed(()=>[...credentialIdentities.value].sort((a,b)=>a.name.localeCompare(b.name,'de')))
function openRow(event:MouseEvent,id:string) { const href=rowDestination(event,`/system/identitaeten/${id}`);if(href)void router.push(href) }
async function duplicate(identity:CredentialIdentity) { busy.value=true;try { await saveCredentialIdentity({name:identity.name+' Kopie',username:identity.username,copy_password_from_id:identity.id}) } catch(err) { error.value=err instanceof Error?err.message:'Duplizieren fehlgeschlagen.' } finally { busy.value=false } }
onMounted(()=>void loadAll())
</script>
