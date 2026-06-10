<template>
  <header class="topline">
    <div>
      <div class="eyebrow">System</div>
      <h1 class="headline">{{ activeLabel }}</h1>
    </div>
    <div class="meta">
      <div>Version · <b>{{ versionLabel }}</b></div>
      <div>Adresse · <b>{{ settings.bind_addr || '127.0.0.1:8091' }}</b></div>
    </div>
  </header>

  <div v-if="error" class="notice err"><span class="tag">FEHLER</span>{{ error }}</div>

  <nav class="subtabs">
    <RouterLink v-for="tab in tabs" :key="tab.to" :to="tab.to" class="subtab">{{ tab.label }}</RouterLink>
  </nav>

  <RouterView />

  <div class="toast-host">
    <transition name="page"><div v-if="toast" class="toast" :key="toast">{{ toast }}</div></transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useSystem } from '../composables/useSystem'

const route = useRoute()
const { settings, error, toast, versionLabel, loadAll } = useSystem()

const tabs = [
  { to: '/system/allgemein', label: 'Allgemein' },
  { to: '/system/zugriff', label: 'Zugriff' },
  { to: '/system/netzwerk', label: 'Netzwerk & Relay' },
  { to: '/system/identitaeten', label: 'Identitäten' },
  { to: '/system/wartung', label: 'Wartung' }
]
const activeLabel = computed(() => tabs.find((tab) => route.path.startsWith(tab.to))?.label || 'System')

onMounted(() => void loadAll())
</script>

<style scoped>
.subtabs { display: flex; flex-wrap: wrap; gap: 4px; }
.subtab {
  padding: 8px 14px;
  border-radius: 999px;
  font-size: 11px;
  letter-spacing: .06em;
  text-transform: uppercase;
  color: var(--ink-mute);
  transition: color .12s ease, background .12s ease;
}
.subtab:hover { color: var(--ink); background: var(--surface); }
.subtab.router-link-active { color: var(--bg); background: var(--ink); }
</style>
