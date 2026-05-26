<template>
  <button class="action-button" :class="variant" :disabled="disabled || busy">
    {{ busy ? 'Bitte warten...' : label }}
  </button>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ label: string; variant?: 'primary' | 'secondary' | 'danger'; busy?: boolean; disabled?: boolean }>(), {
  variant: 'secondary',
  busy: false,
  disabled: false
})
</script>
