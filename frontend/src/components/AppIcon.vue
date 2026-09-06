<template>
  <svg class="app-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"><path v-for="path in icons[name]" :key="path" :d="path" /></svg>
</template>
<script setup lang="ts">
import { icons, type IconName } from '../icons'
defineProps<{ name: IconName }>()
</script>
<style scoped>
.app-icon { width:20px;height:20px;flex-shrink:0; }
</style>
