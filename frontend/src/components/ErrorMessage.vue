<template>
  <div v-if="message" class="error-message">{{ message }}</div>
</template>

<script setup lang="ts">
defineProps<{ message?: string }>()
</script>
