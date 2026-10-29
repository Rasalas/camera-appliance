<template>
  <details @pointerdown.stop @click.stop>
    <summary>Anordnen</summary>
    <label>Ziel <select v-model="target" :aria-label="`${label}: Zielkamera`"><option value="" disabled>Kamera wählen</option><option v-for="peer in peers.filter(p => p.alias !== alias)" :key="peer.alias" :value="peer.alias">{{ peer.label }}</option></select></label>
    <label>Position <select v-model="side" :aria-label="`${label}: Anordnung`"><option value="center">Tauschen</option><option value="left">Links</option><option value="right">Rechts</option><option value="top">Darüber</option><option value="bottom">Darunter</option></select></label>
    <button class="btn sm" :disabled="!target" @click="$emit('dock',target,side)">Anordnen</button>
  </details>
</template>
<script setup lang="ts">
import { ref } from 'vue'
import type { Side } from '../pages/viewerMosaic'
defineProps<{alias: string; label: string; peers: {alias:string;label:string}[]}>()
defineEmits<{dock: [target: string, side: Side]}>()
const target = ref(''), side = ref<Side>('center')
</script>
