<template>
  <header class="topline"><h1 class="headline">{{ activeLabel }}</h1></header>

  <div v-if="error" class="notice err"><span class="tag">FEHLER</span>{{ error }}</div>

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
const { error, toast, loadAll, credentialIdentities, relayName } = useSystem()

const activeLabel = computed(() => route.params.id && route.path.includes('/identitaeten/') ? credentialIdentities.value.find(item => item.id === route.params.id)?.name || 'Identität' : route.params.id && route.path.includes('/relays/') ? relayName(String(route.params.id)) : String(route.meta.title || 'System'))

onMounted(() => void loadAll())
</script>
