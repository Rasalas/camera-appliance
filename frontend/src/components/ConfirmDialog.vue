<template>
  <div v-if="open" class="dialog-backdrop">
    <div class="dialog">
      <h3>{{ title }}</h3>
      <p>{{ message }}</p>
      <div class="button-row">
        <button class="action-button danger" @click="$emit('confirm')">Bestätigen</button>
        <button class="action-button secondary" @click="$emit('cancel')">Abbrechen</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ open: boolean; title: string; message: string }>()
defineEmits<{ confirm: []; cancel: [] }>()
</script>
