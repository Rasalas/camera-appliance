<template>
  <div class="slot-list">
    <label v-for="slot in slots" :key="slot.id" class="slot-option">
      <input type="radio" name="slot" :value="slot.id" :checked="modelValue === slot.id" @change="$emit('update:modelValue', slot.id)" />
      <span>{{ slot.id }} - {{ slot.label }}</span>
    </label>
  </div>
</template>

<script setup lang="ts">
import type { Slot } from '../types'

defineProps<{ slots: Slot[]; modelValue: string }>()
defineEmits<{ 'update:modelValue': [value: string] }>()
</script>
