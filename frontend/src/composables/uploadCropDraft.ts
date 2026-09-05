import type { UploadCrop } from '../types/index.ts'

export function validUploadCrop(crop: UploadCrop): boolean {
  return !crop.enabled || ([crop.x, crop.y, crop.width, crop.height].every(Number.isFinite)
    && crop.x >= 0 && crop.y >= 0 && crop.width > 0 && crop.height > 0
    && crop.x + crop.width <= 100 && crop.y + crop.height <= 100)
}

type SaveState = 'pending' | 'saving' | 'saved' | 'error'

// Serialize saves and retain only the latest draft. A slow response must never
// replace a newer edit, and leaving the page flushes the final valid selection.
export function createCropAutosave(
  save: (crop: UploadCrop) => Promise<unknown>,
  report: (state: SaveState, error?: unknown) => void,
  delay = 450
) {
  let pending: { crop: UploadCrop; revision: number } | undefined
  let revision = 0
  let timer: ReturnType<typeof setTimeout> | undefined
  let running: Promise<void> | undefined
  let closed = false
  const notify = (state: SaveState, error?: unknown) => { if (!closed) report(state, error) }

  function cancelPending() {
    clearTimeout(timer)
    pending = undefined
    revision++
  }

  function flush(): Promise<void> {
    clearTimeout(timer)
    if (running) return running
    if (!pending) return Promise.resolve()
    running = (async () => {
      let savedRevision = -1
      while (pending) {
        clearTimeout(timer)
        const draft = pending
        pending = undefined
        notify('saving')
        try { await save(draft.crop); savedRevision = draft.revision }
        catch (err) {
          // A subsequent edit retries; never loop on a failed request.
          if (!pending) {
            if (draft.revision === revision) { pending = draft; notify('error', err) }
            return
          }
        }
      }
      if (savedRevision === revision) notify('saved')
    })().finally(() => { running = undefined })
    return running
  }

  return {
    change(crop: UploadCrop) {
      cancelPending()
      if (closed || !validUploadCrop(crop)) return
      pending = { crop: { ...crop }, revision }
      notify('pending')
      timer = setTimeout(() => void flush(), delay)
    },
    cancelPending,
    flush,
    close() { closed = true; return flush() }
  }
}
