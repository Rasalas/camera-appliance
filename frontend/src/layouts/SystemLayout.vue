<template>
  <header class="topline"><h1 class="headline">{{ activeLabel }}</h1><div id="system-page-actions" class="page-actions" /></header>

  <div v-if="loadError" class="notice err load-error" role="alert"><span class="tag">FEHLER</span><span>{{ loadError }}</span><button class="btn" type="button" :disabled="retrying" @click="retry">{{ retrying ? 'Lädt…' : 'Erneut versuchen' }}</button></div>
  <div v-if="error" class="notice err"><span class="tag">FEHLER</span>{{ error }}</div>

  <RouterView />

  <div class="toast-host">
    <transition name="page"><div v-if="toast" class="toast" :key="toast">{{ toast }}</div></transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useSystem } from '../composables/useSystem'

const route = useRoute()
const { error, loadError, toast, loadAll, credentialIdentities, relayName } = useSystem()
const retrying = ref(false)

const activeLabel = computed(() => route.params.id && route.path.includes('/identitaeten/') ? credentialIdentities.value.find(item => item.id === route.params.id)?.name || 'Kamera-Zugang' : route.params.id && route.path.includes('/relays/') ? relayName(String(route.params.id)) : String(route.meta.title || 'System'))

async function retry() { retrying.value = true; try { await loadAll() } finally { retrying.value = false } }

async function retry() { retrying.value = true; try { await loadAll() } finally { retrying.value = false } }

onMounted(() => void loadAll())
</script>

<style scoped>
.load-error { align-items: center; }
.load-error > span:not(.tag) { flex: 1; }
</style>
