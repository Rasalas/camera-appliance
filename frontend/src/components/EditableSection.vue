<template>
  <section class="panel edit-section" :class="{ 'section-clickable': !disabled }" :aria-labelledby="headingId" @click="openFromSurface">
    <div class="panel-head">
      <h2 :id="headingId">{{ title }}</h2>
      <RouterLink v-if="to && !disabled" class="btn ghost section-edit-action" :to="to" :aria-label="title + ' bearbeiten'"><AppIcon name="edit" />Bearbeiten</RouterLink>
      <button v-else class="btn ghost section-edit-action" type="button" :disabled="disabled" :aria-label="title + ' bearbeiten'" @click="$emit('edit')"><AppIcon name="edit" />Bearbeiten</button>
    </div>
    <slot />
  </section>
</template>
<script setup lang="ts">
import { useId } from 'vue'
import { rowDestination } from '../composables/resourceRow'
import AppIcon from './AppIcon.vue'
const props = defineProps<{ title: string; to?: string; disabled?: boolean }>()
defineEmits<{ edit: [] }>()
const headingId = useId()
function openFromSurface(event: MouseEvent) {
  if (props.disabled || !rowDestination(event, 'edit')) return
  const action = (event.currentTarget as HTMLElement).querySelector<HTMLElement>('.section-edit-action')
  action?.focus({ preventScroll: true })
  action?.click()
}
</script>
