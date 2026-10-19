<template>
  <header class="topline">
    <h1 class="headline">{{ initialSetup ? 'Admin-Passwort festlegen' : 'Anmelden' }}</h1>
  </header>

  <div v-if="unreachable" class="notice warn" role="status"><span class="tag">VERBINDUNG</span>Station nicht erreichbar – neuer Versuch läuft.</div>
  <div v-if="error" class="notice err"><span class="tag">FEHLER</span>{{ error }}</div>

  <section class="panel card login-panel">
    <p v-if="statusLine" class="mono-mute">{{ statusLine }}</p>

    <form v-if="initialSetup" class="login-form" @submit.prevent="setInitialPassword">
      <p class="mono-mute">Lege ein Passwort für die Verwaltung fest. Danach ist die Verwaltung nur noch mit Anmeldung erreichbar.</p>
      <div class="field">
        <span class="lbl">Neues Admin-Passwort</span>
        <input aria-label="Neues Admin-Passwort" v-model="newPassword" type="password" autocomplete="new-password" autofocus />
      </div>
      <div class="field">
        <span class="lbl">Bestätigung</span>
        <input aria-label="Bestätigung" v-model="confirmPassword" type="password" autocomplete="new-password" />
      </div>
      <div class="btn-row">
        <button class="btn primary" type="submit" :disabled="busy || !newPassword || !confirmPassword">
          {{ busy ? 'Speichert…' : 'Passwort setzen' }}
        </button>
        <RouterLink class="btn ghost" to="/">Kameras öffnen</RouterLink>
      </div>
    </form>

    <form v-else class="login-form" @submit.prevent="login">
      <div v-if="auth?.viewer_password_set" class="field">
        <span class="lbl">Zugang</span>
        <select aria-label="Zugang" v-model="username" name="username" autocomplete="username">
          <option value="admin">Verwaltung (Admin)</option>
          <option value="viewer">Nur Kameras ansehen (Viewer)</option>
        </select>
      </div>
      <div class="field">
        <span class="lbl">{{ auth?.viewer_password_set ? 'Passwort' : 'Admin-Passwort' }}</span>
        <input aria-label="Passwort" v-model="password" name="password" type="password" autocomplete="current-password" autofocus />
      </div>
      <label class="toggle-row compact">
        <input v-model="remember" type="checkbox" />
        <div><div class="lbl-main">Angemeldet bleiben</div><div class="lbl-sub">{{ rememberHint }}</div></div></label>
      <div class="btn-row">
        <button class="btn primary" type="submit" :disabled="busy || !password">
          {{ busy ? 'Prüft…' : 'Anmelden' }}
        </button>
        <RouterLink class="btn ghost" to="/">Kameras öffnen</RouterLink>
      </div>
    </form>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import { authDecision, loginTarget } from '../router/authGuard'
import type { AuthRole, AuthStatus } from '../types'

const route = useRoute()
const router = useRouter()
const auth = ref<AuthStatus>()
const username = ref<AuthRole>('admin')
const password = ref('')
const remember = ref(false)
const newPassword = ref('')
const confirmPassword = ref('')
const busy = ref(false)
const error = ref('')
const unreachable = ref(false)
let retryTimer: number | undefined
let attempts = 0
let disposed = false

const initialSetup = computed(() => !!auth.value && !auth.value.admin_password_set)
const statusLine = computed(() => {
  if (!auth.value) return unreachable.value ? '' : 'Anmeldestatus wird geladen…'
  if (auth.value.role === 'admin') return 'Du bist bereits als Admin angemeldet.'
  if (auth.value.role === 'viewer') return 'Du bist als Viewer angemeldet. Für die Verwaltung bitte als Admin anmelden.'
  if (auth.value.viewer_public) return 'Die Kameras sind auch ohne Anmeldung sichtbar. Die Anmeldung ist nur für die Verwaltung nötig.'
  return ''
})
const rememberHint = computed(() => `Auf diesem Gerät 30 Tage lang angemeldet bleiben. Sonst endet die Anmeldung nach ${auth.value?.session_hours || 12} Stunden.`)

function nextPathFor(role: AuthRole) {
  const next = loginTarget(route.query.next) ?? '/'
  if (role !== 'admin' && isAdminPath(next)) return '/'
  return next
}

function isAdminPath(path: string) {
  return ['/einrichtung', '/uebersicht', '/system', '/kamera', '/setup', '/overview', '/settings', '/events', '/backup'].some((prefix) => path === prefix || path.startsWith(prefix + '/'))
}

// Pages reached via a redirect go back on their own once access is possible again.
function returnIfAllowed(status: AuthStatus) {
  const next = loginTarget(route.query.next)
  if (!next) return false
  if (authDecision(router.resolve(next).meta, next, status) !== true) return false
  void router.replace(next)
  return true
}

async function loadAuth() {
  retryTimer = undefined
  try {
    const status = await api.authStatus()
    if (disposed) return
    auth.value = status
    unreachable.value = false
    attempts = 0
    if (!status.viewer_password_set && username.value === 'viewer') username.value = 'admin'
    returnIfAllowed(status)
  } catch {
    if (disposed) return
    unreachable.value = true
    attempts++
    retryTimer = window.setTimeout(loadAuth, Math.min(1000 * 2 ** (attempts - 1), 15000))
  }
}

async function login() {
  busy.value = true
  error.value = ''
  try {
    const result = await api.login({ username: username.value, password: password.value, remember: remember.value })
    password.value = ''
    window.dispatchEvent(new Event('auth-changed'))
    await router.push(nextPathFor(result.role))
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Anmeldung fehlgeschlagen.'
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

onMounted(() => void loadAuth())
onBeforeUnmount(() => {
  disposed = true
  if (retryTimer !== undefined) window.clearTimeout(retryTimer)
})
</script>

<style scoped>
.login-panel {
  max-width: 560px;
}

.login-panel > p:first-child { margin-top: 0; }

.login-form {
  display: grid;
  gap: 16px;
}
</style>
