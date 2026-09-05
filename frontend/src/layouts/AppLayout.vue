<template>
  <div class="shell" :class="{ 'shell-bleed': isViewer }">
    <aside v-if="!isViewer" class="rail">
      <div class="brand">
        <span class="mark" />
        <span class="name">Watch<em>deck</em></span>
      </div>

      <nav class="nav">
        <RouterLink to="/">Kameras<span class="nav-key">1</span></RouterLink>
        <RouterLink v-if="canAdmin" to="/einrichtung">Geräte<span class="nav-key">2</span></RouterLink>
        <RouterLink v-if="canAdmin" to="/system">System<span class="nav-key">3</span></RouterLink>
      </nav>

      <!-- Pinned bottom block: update control, metadata rows, then the auth
           action. On narrow screens the control moves up into the brand row. -->
      <div class="rail-bottom">
        <div class="rail-tray">
          <UpdatePanel :visible="canAdmin" />
        </div>

        <div class="rail-foot">
          <div class="row"><span>Stand</span><b>{{ clock }}</b></div>
          <div class="row"><span>Login</span><b>{{ roleLabel }}</b></div>
          <div class="row"><span>Bind</span><b>127.0.0.1:8091</b></div>
        </div>

        <div class="auth-actions">
          <button v-if="auth?.enabled && auth.authenticated" class="btn sm ghost rail-login" type="button" @click="logout">Logout</button>
          <RouterLink v-else-if="auth?.enabled" class="btn sm ghost rail-login" to="/login">Login</RouterLink>
        </div>
      </div>
    </aside>

    <main class="canvas">
      <RouterView v-slot="{ Component, route }">
        <div :key="route.fullPath" class="route-fade">
          <component :is="Component" />
        </div>
      </RouterView>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import type { AuthStatus } from '../types'
import UpdatePanel from '../components/UpdatePanel.vue'

const router = useRouter()
const route = useRoute()
const clock = ref('')
const auth = ref<AuthStatus>()
const isViewer = computed(() => route.name === 'viewer')

const canAdmin = computed(() => auth.value ? (!auth.value.enabled || auth.value.role === 'admin') : false)
const roleLabel = computed(() => {
  if (!auth.value?.enabled) return 'offen'
  if (auth.value.role === 'admin') return auth.value.local_admin_bypass_now ? 'Host' : 'Admin'
  if (auth.value.role === 'viewer') return 'Viewer'
  return 'gesperrt'
})

function tick() {
  clock.value = new Date().toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
let timer = 0
let onKey: ((e: KeyboardEvent) => void) | undefined
let onAuthChanged: (() => void) | undefined
let removeAfterEach: (() => void) | undefined

async function refreshAuth() {
  try {
    auth.value = await api.authStatus()
  } catch {
    auth.value = undefined
  }
}

async function logout() {
  await api.logout()
  window.dispatchEvent(new Event('auth-changed'))
  await refreshAuth()
  await router.push('/')
}

onMounted(() => {
  void refreshAuth()
  removeAfterEach = router.afterEach(() => {
    void refreshAuth()
  })
  onAuthChanged = () => {
    void refreshAuth()
  }
  window.addEventListener('auth-changed', onAuthChanged)
  tick()
  timer = window.setInterval(tick, 1000)
  onKey = (e: KeyboardEvent) => {
    if (e.target instanceof HTMLElement && ['INPUT', 'TEXTAREA', 'SELECT'].includes(e.target.tagName)) return
    if (e.key === '1') router.push('/')
    if (canAdmin.value && e.key === '2') router.push('/einrichtung')
    if (canAdmin.value && e.key === '3') router.push('/system')
  }
  window.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => {
  window.clearInterval(timer)
  if (onKey) window.removeEventListener('keydown', onKey)
  if (onAuthChanged) window.removeEventListener('auth-changed', onAuthChanged)
  if (removeAfterEach) removeAfterEach()
})
</script>

<style scoped>
.route-fade {
  display: flex;
  flex-direction: column;
  gap: var(--gutter);
  animation: route-in .18s ease-out both;
}
/* Viewer route: edge-to-edge, no rail, no canvas chrome. */
.shell-bleed {
  grid-template-columns: 1fr;
  min-height: 100vh;
  min-height: 100dvh;
}
.shell-bleed :deep(.canvas) {
  padding: 0;
  max-width: none;
  gap: 0;
  min-height: 100vh;
  min-height: 100dvh;
}
.shell-bleed .route-fade { gap: 0; }
@keyframes route-in {
  from { opacity: 0; }
  to   { opacity: 1; }
}
.rail-login {
  width: 100%;
}
.auth-actions {
  display: grid;
  min-width: 0;
}
</style>
