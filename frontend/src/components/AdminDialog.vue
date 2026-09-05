<template>
  <dialog ref="dialog" class="admin-dialog" :class="{ 'discard-dialog': compact }" :aria-label="title" @cancel.prevent="requestClose" @click="outside" @touchstart.passive="touchStart" @touchmove="touchMove" @touchend="touchEnd" @touchcancel="resetDrag">
    <div class="dialog-surface" :style="{ transform: `translateY(${dragY}px)` }">
      <header class="modal-head"><h2>{{ title }}</h2><button class="btn icon ghost" type="button" aria-label="Schließen" :disabled="busy" @click="requestClose"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18" /></svg></button></header>
      <div ref="body" class="dialog-body"><slot /></div>
    </div>
  </dialog>
</template>
<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { askDiscard } from '../composables/discardChanges'
const props = defineProps<{ open: boolean; title: string; dirty?: boolean; busy?: boolean; compact?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement>(), body = ref<HTMLElement>()
const dragY = ref(0)
let returnFocus: HTMLElement | undefined, previousOverflow = '', startY = 0, lastY = 0, startTime = 0, eligible = false
watch(() => props.open, async open => {
  await nextTick()
  if (open && !dialog.value?.open) {
    returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
    previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    dialog.value?.showModal()
    dialog.value?.querySelector<HTMLElement>('[autofocus], input:not([type=hidden]), select, textarea')?.focus()
  } else if (!open && dialog.value?.open) {
    dialog.value.close()
    document.body.style.overflow = previousOverflow
    returnFocus?.focus()
  }
}, { immediate: true })
async function requestClose() {
  if (props.busy || (props.dirty && !await askDiscard())) return
  emit('close')
}
function outside(event: MouseEvent) { if (event.target === dialog.value) void requestClose() }
function touchStart(event: TouchEvent) {
  const target = event.target as HTMLElement
  eligible = !props.compact && innerWidth <= 820 && event.touches.length === 1 && !target.closest('input,textarea,select,button,a,[role=slider]') && !window.getSelection()?.toString()
  startY = lastY = event.touches[0]?.clientY ?? 0
  startTime = performance.now()
}
function touchMove(event: TouchEvent) {
  if (!eligible || !event.touches[0]) return
  const y = event.touches[0].clientY, delta = y - lastY
  lastY = y
  // Own scrolling on free content surfaces so the same gesture can hand off
  // at scrollTop=0. Inputs and inner interactive controls keep native handling.
  const scroll = (event.target as HTMLElement).closest('.dialog-body') ? body.value : undefined
  if (dragY.value === 0 && scroll && (scroll.scrollTop > 0 || delta < 0)) {
    const remaining = Math.max(0, delta - scroll.scrollTop)
    scroll.scrollTop -= delta
    if (remaining) { dragY.value = remaining; startY = y - remaining; startTime = performance.now() }
  } else dragY.value = Math.max(0, dragY.value + delta)
  if (event.cancelable) event.preventDefault()
}
function resetDrag() { dragY.value = 0; eligible = false }
function touchEnd() {
  // 110 px travel or a deliberate 45 px flick above 0.65 px/ms.
  const close = eligible && (dragY.value > 110 || (dragY.value > 45 && (lastY - startY) / Math.max(1, performance.now() - startTime) > .65))
  resetDrag()
  if (close) void requestClose()
}
onBeforeUnmount(() => { if (dialog.value?.open) { dialog.value.close(); document.body.style.overflow = previousOverflow } })
defineExpose({ requestClose })
</script>
