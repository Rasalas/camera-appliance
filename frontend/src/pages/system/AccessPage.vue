<template>
  <SettingsForm title="Netzwerkzugriff" :setting-keys="['network.lan_access_enabled']" :after-save="applyNetwork">
    <template #summary><p>{{ settings['network.lan_access_enabled'] === 'true' ? 'Die Station ist im lokalen Netzwerk erreichbar.' : 'Zugriff ist auf dieses Gerät beschränkt.' }}</p><p class="mono-mute">Eine Änderung startet den Dienst neu.</p></template>
    <label class="toggle-row"><input type="checkbox" :checked="settings['network.lan_access_enabled'] === 'true'" :disabled="settings.auth_admin_password_set !== 'true'" @change="setBool('network.lan_access_enabled',$event)" /><span>Zugriff aus dem lokalen Netzwerk erlauben</span></label>
    <p v-if="settings.auth_admin_password_set !== 'true'" class="notice warn">Setze zuerst ein Admin-Passwort, um den Netzwerkzugriff freizugeben.</p>
    <p class="mono-mute">Speichern wendet den Netzwerkzugriff an und startet die Station neu.</p>
  </SettingsForm>
  <section v-for="item in roles" :key="item.id" class="panel edit-section"><div class="panel-head"><h2>{{ item.label }}</h2><button class="btn ghost" @click="openPassword(item.id)">Bearbeiten</button></div><p>{{ settings[`auth_${item.id}_password_set`] === 'true' ? 'Passwort gespeichert.' : 'Noch kein Passwort gesetzt.' }}</p></section>
  <SettingsForm title="Anmeldung" :setting-keys="loginKeys">
    <template #summary><dl class="spec"><div><dt>Sitzungsdauer</dt><dd>{{ settings['auth.session_hours'] }} Stunden</dd></div><div><dt>Live-Ansicht ohne Login</dt><dd>{{ settings['auth.viewer_public'] === 'true' ? 'Erlaubt' : 'Gesperrt' }}</dd></div><div><dt>Lokaler Admin-Zugriff</dt><dd>{{ settings['auth.local_admin_bypass'] === 'true' ? 'Erlaubt' : 'Login erforderlich' }}</dd></div></dl></template>
    <label class="field"><span class="lbl">Sitzungsdauer · Stunden</span><input aria-label="Sitzungsdauer · Stunden" v-model="settings['auth.session_hours']" type="number" min="1" max="168" required /><span class="mono-mute">„Angemeldet bleiben“ merkt dieses Gerät unabhängig davon 30 Tage lang.</span></label>
    <label class="toggle-row"><input type="checkbox" :checked="settings['auth.viewer_public'] === 'true'" @change="setBool('auth.viewer_public',$event)" /><span>Live-Ansicht ohne Login erlauben</span></label>
    <label class="toggle-row"><input type="checkbox" :checked="settings['auth.local_admin_bypass'] === 'true'" @change="setBool('auth.local_admin_bypass',$event)" /><span>Lokalen Host als Admin akzeptieren</span></label>
  </SettingsForm>
  <AdminDialog ref="dialog" :open="!!role" :title="role === 'admin' ? 'Admin-Passwort bearbeiten' : 'Viewer-Passwort bearbeiten'" :dirty="!!password" :busy="saving" @close="closePassword"><form @submit.prevent="savePassword"><label class="field"><span class="lbl">Neues Passwort · Pflichtfeld</span><input aria-label="Neues Passwort · Pflichtfeld" v-model="password" type="password" required autocomplete="new-password" autofocus /></label><p v-if="passwordError" class="notice err" role="alert">{{ passwordError }}</p><div class="form-actions"><button class="btn" type="button" @click="dialog?.requestClose()">Abbrechen</button><button class="btn primary" :disabled="saving" type="submit">Passwort speichern</button></div></form></AdminDialog>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../../api/client'
import { useSystem } from '../../composables/useSystem'
import { useDraftGuard } from '../../composables/discardChanges'
import SettingsForm from '../../components/SettingsForm.vue'
import AdminDialog from '../../components/AdminDialog.vue'
import type { AuthRole } from '../../types'
const {settings,setBool,loadAll,saveAuthPassword}=useSystem()
const roles=[{id:'admin' as const,label:'Admin-Login'},{id:'viewer' as const,label:'Viewer-Login'}]
const loginKeys=['auth.viewer_public','auth.local_admin_bypass','auth.session_hours']
const role=ref<AuthRole | ''>(''),password=ref(''),saving=ref(false),passwordError=ref(''),dialog=ref<InstanceType<typeof AdminDialog>>()
function openPassword(value:AuthRole) { role.value=value;password.value='';passwordError.value='' }
function closePassword() { role.value='';password.value='';passwordError.value='' }
useDraftGuard(()=>!!role.value && !!password.value,closePassword)
async function savePassword() { if(!role.value || saving.value)return;saving.value=true;passwordError.value='';try {await saveAuthPassword(role.value,password.value);closePassword()}catch(err){passwordError.value=err instanceof Error?err.message:'Passwort konnte nicht gespeichert werden.'}finally{saving.value=false} }
async function applyNetwork() { void api.restartStack().catch(()=>undefined);window.setTimeout(()=>window.location.reload(),10000) }
onMounted(()=>void loadAll())
</script>
