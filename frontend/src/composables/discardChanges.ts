import { onBeforeUnmount } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router'
import { ref } from 'vue'

export const discardOpen = ref(false)
const draftChecks = new Set<() => boolean>()
// Lets unattended flows (kiosk idle return) skip navigation instead of prompting.
export function hasUnsavedDrafts() { return discardOpen.value || [...draftChecks].some(dirty => dirty()) }
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
export function useDraftGuard(dirty: () => boolean, discard: () => void | Promise<void>, save?: () => Promise<boolean>) {
  draftChecks.add(dirty)
  const guard = async () => {
    if (!dirty()) return true
    if (save && await save().catch(() => false)) return true
    if (!await askDiscard()) return false
    await discard()
    return true
  }
  onBeforeRouteLeave(guard)
  // AppLayout keys pages by fullPath, so hash/query changes also remount drafts.
  onBeforeRouteUpdate((to, from) => to.fullPath === from.fullPath ? true : guard())
  const unload = (event: BeforeUnloadEvent) => {
    if (dirty()) { event.preventDefault(); event.returnValue = '' }
  }
  window.addEventListener('beforeunload', unload)
  onBeforeUnmount(() => { window.removeEventListener('beforeunload', unload); draftChecks.delete(dirty) })
}
