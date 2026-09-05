<template>
  <div class="shell" :class="{ 'shell-bleed': isViewer }">
    <aside v-if="!isViewer" class="rail">
      <div class="brand">
        <span class="mark" />
        <span class="name">Watch<em>deck</em></span>
      </div>

      <AdminSearch v-if="canAdmin" />
      <div class="mobile-overflow" v-if="auth?.enabled">
        <button ref="overflowTrigger" class="btn icon ghost" aria-label="Weitere Aktionen" :aria-expanded="overflowOpen" @click="overflowOpen = !overflowOpen">⋮</button>
        <div v-if="overflowOpen" ref="overflowPopup" class="overflow-popover" @keydown.esc="closeOverflow">
          <button v-if="auth.authenticated" class="btn ghost" @click="logout">Logout</button>
          <RouterLink v-else class="btn" to="/login">Login</RouterLink>
        </div>
      </div>
      <nav class="nav" aria-label="Hauptnavigation">
        <RouterLink to="/verwaltung">Home</RouterLink>
        <RouterLink to="/">Live-Ansicht<span class="nav-key">1</span></RouterLink>
        <div v-if="canAdmin" class="nav-group">
          <RouterLink to="/einrichtung">Kameras<span class="nav-key">2</span></RouterLink>
          <div class="nav-children" aria-label="Kameras">
            <RouterLink to="/kameras/bild-upload">Bild-Upload</RouterLink>
          </div>
        </div>
        <div v-if="canAdmin" class="nav-group">
          <RouterLink to="/system/allgemein">System<span class="nav-key">3</span></RouterLink>
          <div class="nav-children" aria-label="System">
            <RouterLink v-for="item in systemPages" :key="item.to" :to="item.to">{{ item.label }}</RouterLink>
          </div>
        </div>
        <div v-if="canAdmin" class="nav-group">
          <RouterLink to="/system/wartung">Wartung</RouterLink>
          <div class="nav-children" aria-label="Wartung">
            <RouterLink v-for="item in maintenancePages" :key="item.to" :to="item.to">{{ item.label }}</RouterLink>
          </div>
        </div>
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

        </div>

        <div v-if="canAdmin" class="rail-version">Version {{ versionLabel }}</div>
        <div class="auth-actions">
          <button v-if="auth?.enabled && auth.authenticated" class="btn sm ghost rail-login" type="button" @click="logout">Logout</button>
          <RouterLink v-else-if="auth?.enabled" class="btn sm ghost rail-login" to="/login">Login</RouterLink>
        </div>
      </div>
    </aside>

    <DiscardChanges />
    <main class="canvas" :class="{ 'admin-canvas': !isViewer }">
      <RouterView v-slot="{ Component, route }">
        <div :key="route.fullPath" class="route-fade">
          <component :is="Component" />
        </div>
      </RouterView>
    </main>
    <nav v-if="!isViewer && canAdmin" class="bottom-navigation" aria-label="Mobile Hauptnavigation"><RouterLink to="/verwaltung">Home</RouterLink><RouterLink to="/einrichtung">Kameras</RouterLink><RouterLink to="/">Live-Ansicht</RouterLink></nav>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import type { AuthStatus } from '../types'
import UpdatePanel from '../components/UpdatePanel.vue'
import AdminSearch from '../components/AdminSearch.vue'
import DiscardChanges from '../components/DiscardChanges.vue'
import { systemPages, maintenancePages } from '../navigation'
import { provideUpdateFlow } from '../composables/useUpdateFlow'

const router = useRouter()
const route = useRoute()
provideUpdateFlow()
const versionLabel = ref('…')
const overflowOpen = ref(false), overflowTrigger = ref<HTMLElement>(), overflowPopup = ref<HTMLElement>()
function closeOverflow() { overflowOpen.value = false; overflowTrigger.value?.focus() }
watch(overflowOpen, async open => { if (open) { await nextTick(); overflowPopup.value?.querySelector<HTMLElement>('button,a')?.focus() } })
watch(() => route.fullPath, () => { overflowOpen.value = false })
function outsideOverflow(event: PointerEvent) { if (overflowOpen.value && !overflowPopup.value?.contains(event.target as Node) && !overflowTrigger.value?.contains(event.target as Node)) closeOverflow() }
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
    if (canAdmin.value) await api.health().then(value => { versionLabel.value = value.version }).catch(() => undefined)
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
    if (document.querySelector('dialog[open]') || (e.target as HTMLElement)?.closest('.search-popover')) return
    if (e.target instanceof HTMLElement && ['INPUT', 'TEXTAREA', 'SELECT'].includes(e.target.tagName)) return
    if (e.key === '1') router.push('/')
    if (canAdmin.value && e.key === '2') router.push('/einrichtung')
    if (canAdmin.value && e.key === '3') router.push('/system')
  }
  window.addEventListener('keydown', onKey)
  window.addEventListener('pointerdown', outsideOverflow)
})
onBeforeUnmount(() => {
  window.removeEventListener('pointerdown', outsideOverflow)
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
.nav { min-height:0;overflow-y:auto; }
.nav-group { display:grid;gap:2px; }
.nav-children { margin:0 0 10px 16px; }
.nav-children a { padding:8px 6px; }
.rail-version { font-size:12px;color:var(--ink-mute); }
</style>
