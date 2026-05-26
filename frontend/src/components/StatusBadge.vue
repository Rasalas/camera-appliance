<template>
  <span class="status-badge" :class="online ? 'ok' : 'bad'">
    {{ online ? 'Online' : 'Offline' }}
  </span>
</template>

<script setup lang="ts">
defineProps<{ online: boolean }>()
</script>
