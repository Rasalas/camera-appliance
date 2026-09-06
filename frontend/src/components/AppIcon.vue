<template>
  <component :is="icons[name]" class="app-icon" :size="20" :stroke-width="2" aria-hidden="true" focusable="false" />
</template>
<script setup lang="ts">
import { icons, type IconName } from '../icons'
defineProps<{ name: IconName }>()
</script>
<style scoped>
.app-icon { width:20px;height:20px;flex-shrink:0; }
</style>
