<template>
  <header class="topline">
    <div>
      <div class="eyebrow">Zugriff · Lokaler Login</div>
      <h1 class="headline">Login.</h1>
    </div>
    <div class="meta">
      <div>Viewer · <b>{{ auth?.viewer_public ? 'öffentlich' : 'geschützt' }}</b></div>
      <div>Session · <b>{{ auth?.session_hours || 12 }} h</b></div>
    </div>
  </header>

  <div v-if="error" class="notice err"><span class="tag">FEHLER</span>{{ error }}</div>

  <section class="panel card login-panel">
    <div class="panel-head">
      <h2>{{ initialSetup ? 'Admin-Passwort setzen' : 'Anmelden' }}</h2>
      <div class="right">{{ authLabel }}</div>
    </div>

    <form v-if="initialSetup" class="login-form" @submit.prevent="setInitialPassword">
      <div class="field">
        <span class="lbl">Neues Admin-Passwort</span>
        <input v-model="newPassword" type="password" autocomplete="new-password" autofocus />
      </div>
      <div class="field">
        <span class="lbl">Bestätigung</span>
        <input v-model="confirmPassword" type="password" autocomplete="new-password" />
      </div>
      <div class="btn-row">
        <button class="btn primary" type="submit" :disabled="busy || !newPassword || !confirmPassword">
          {{ busy ? 'Speichert…' : 'Passwort setzen' }}
        </button>
      </div>
    </form>

    <form v-else class="login-form" @submit.prevent="login">
      <div class="field">
        <span class="lbl">Rolle</span>
        <select v-model="username">
          <option value="admin">Admin</option>
          <option value="viewer" :disabled="!auth?.viewer_password_set">Viewer</option>
        </select>
      </div>
      <div class="field">
        <span class="lbl">Passwort</span>
        <input v-model="password" type="password" autocomplete="current-password" autofocus />
      </div>
      <div class="btn-row">
        <button class="btn primary" type="submit" :disabled="busy || !password">
          {{ busy ? 'Prüft…' : 'Einloggen' }}
        </button>
        <RouterLink v-if="auth?.viewer_public" class="btn ghost" to="/">Kameras öffnen</RouterLink>
      </div>
    </form>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import type { AuthRole, AuthStatus } from '../types'

const route = useRoute()
const router = useRouter()
const auth = ref<AuthStatus>()
const username = ref<AuthRole>('admin')
const password = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const busy = ref(false)
const error = ref('')

const initialSetup = computed(() => !!auth.value && !auth.value.admin_password_set)
const authLabel = computed(() => {
  if (!auth.value) return 'Lädt'
  if (!auth.value.enabled) return 'Noch offen'
  if (auth.value.role === 'admin') return 'Admin aktiv'
  if (auth.value.role === 'viewer') return 'Viewer aktiv'
  return 'Login erforderlich'
})

function nextPathFor(role: AuthRole) {
  const next = typeof route.query.next === 'string' ? route.query.next : '/'
  if (role !== 'admin' && isAdminPath(next)) return '/'
  return next || '/'
}

function isAdminPath(path: string) {
  return ['/einrichtung', '/uebersicht', '/system', '/kamera', '/setup', '/overview', '/settings', '/events', '/backup'].some((prefix) => path === prefix || path.startsWith(prefix + '/'))
}

async function loadAuth() {
  auth.value = await api.authStatus()
  if (!auth.value.viewer_password_set && username.value === 'viewer') {
    username.value = 'admin'
  }
}

async function login() {
  busy.value = true
  error.value = ''
  try {
    const result = await api.login({ username: username.value, password: password.value })
    password.value = ''
    window.dispatchEvent(new Event('auth-changed'))
    await router.push(nextPathFor(result.role))
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Login fehlgeschlagen.'
  } finally {
    busy.value = false
  }
}

async function setInitialPassword() {
  if (newPassword.value !== confirmPassword.value) {
    error.value = 'Die Bestätigung passt nicht zum Passwort.'
    return
  }
  busy.value = true
  error.value = ''
  try {
    await api.setAuthPassword({ role: 'admin', password: newPassword.value })
    const result = await api.login({ username: 'admin', password: newPassword.value })
    newPassword.value = ''
    confirmPassword.value = ''
    window.dispatchEvent(new Event('auth-changed'))
    await router.push(nextPathFor(result.role))
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Passwort konnte nicht gesetzt werden.'
  } finally {
    busy.value = false
  }
}

onMounted(async () => {
  try {
    await loadAuth()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Login-Status konnte nicht geladen werden.'
  }
})
</script>

<style scoped>
.login-panel {
  max-width: 560px;
}

.login-form {
  display: grid;
  gap: 16px;
}
</style>
