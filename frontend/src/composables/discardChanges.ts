import { onBeforeUnmount } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { ref } from 'vue'

export const discardOpen = ref(false)
let resolveDiscard: ((discard: boolean) => void) | undefined
export function askDiscard(): Promise<boolean> {
  if (resolveDiscard) return Promise.resolve(false)
  discardOpen.value = true
  return new Promise(resolve => { resolveDiscard = resolve })
}
export function finishDiscard(discard: boolean) {
  discardOpen.value = false
  resolveDiscard?.(discard)
  resolveDiscard = undefined
}
export function useDraftGuard(dirty: () => boolean, discard: () => void) {
  onBeforeRouteLeave(async () => {
    if (!dirty()) return true
    if (!await askDiscard()) return false
    discard()
    return true
  })
  const unload = (event: BeforeUnloadEvent) => {
    if (dirty()) { event.preventDefault(); event.returnValue = '' }
  }
  window.addEventListener('beforeunload', unload)
  onBeforeUnmount(() => window.removeEventListener('beforeunload', unload))
}
