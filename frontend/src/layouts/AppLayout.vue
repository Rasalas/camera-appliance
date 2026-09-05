<template>
  <div class="shell" :class="{ 'shell-bleed': isViewer }">
    <aside v-if="!isViewer" class="rail">
      <div class="brand">
        <span class="mark" />
        <span class="name">Watch<em>deck</em></span>
      </div>

      <button ref="menuTrigger" class="nav-toggle btn" type="button" :aria-expanded="menuOpen" aria-controls="main-navigation" @click="menuOpen = !menuOpen">Menü</button>
      <button v-if="menuOpen" class="nav-backdrop" aria-label="Menü schließen" @click="closeMenu" />
      <nav id="main-navigation" ref="navigation" class="nav" :class="{ 'nav-open': menuOpen }" aria-label="Hauptnavigation" @keydown.esc="closeMenu">
        <button class="nav-close btn" type="button" @click="closeMenu">Menü schließen</button>
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
        <div v-if="canAdmin" class="nav-version">Version {{ versionLabel }}</div>
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
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import type { AuthStatus } from '../types'
import UpdatePanel from '../components/UpdatePanel.vue'
import { systemPages, maintenancePages } from '../navigation'
import { provideUpdateFlow } from '../composables/useUpdateFlow'

const router = useRouter()
const route = useRoute()
provideUpdateFlow()
const versionLabel = ref('…')
const menuOpen = ref(false), menuTrigger = ref<HTMLButtonElement>()
const navigation = ref<HTMLElement>()
let previousOverflow = ''
function closeMenu() { menuOpen.value = false; menuTrigger.value?.focus() }
watch(() => route.fullPath, () => { if (menuOpen.value) closeMenu() })
watch(menuOpen, async open => {
  if (open) {
    previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    await nextTick()
    navigation.value?.querySelector<HTMLButtonElement>('button')?.focus()
  } else document.body.style.overflow = previousOverflow
})
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
    if (menuOpen.value) {
      if (e.key === 'Escape') { closeMenu(); return }
      if (e.key === 'Tab') {
        const items = navigation.value?.querySelectorAll<HTMLElement>('a, button')
        const first = items?.[0], last = items?.[items.length - 1]
        if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last?.focus() }
        else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first?.focus() }
      }
      return
    }
    if (e.target instanceof HTMLElement && ['INPUT', 'TEXTAREA', 'SELECT'].includes(e.target.tagName)) return
    if (e.key === '1') router.push('/')
    if (canAdmin.value && e.key === '2') router.push('/einrichtung')
    if (canAdmin.value && e.key === '3') router.push('/system')
  }
  window.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => {
  if (menuOpen.value) document.body.style.overflow = previousOverflow
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
.nav-children { border-left:1px solid var(--hairline-strong);margin:0 0 8px 16px;padding-left:4px; }
.nav-children a { font-size:11px;text-transform:none;letter-spacing:0;padding:7px 6px;grid-template-columns:8px 1fr;gap:6px; }
.rail-version { font-size:10px;color:var(--ink-mute); }
.nav-toggle,.nav-close,.nav-backdrop,.nav-version { display:none; }
@media(max-width:820px) {
  .nav-toggle { display:block;margin-left:auto;margin-right:44px; }
  .nav { display:none; }
  .nav.nav-open { display:flex;flex-direction:column;position:fixed;inset:0 auto 0 0;width:min(320px,85vw);padding:20px;z-index:100;background:var(--surface);grid-template-columns:1fr;align-content:start;gap:8px;overflow-y:auto; }
  .nav-close { display:block;justify-self:end;margin-bottom:12px; }
  .nav-backdrop { display:block;position:fixed;inset:0;z-index:90;border:0;background:#0009; }
  .rail-version { display:none; }
  .nav-version { display:block;margin-top:auto;padding-top:20px;font-size:10px;color:var(--ink-mute); }
  .nav-children a { padding:10px 8px; }
}
</style>
