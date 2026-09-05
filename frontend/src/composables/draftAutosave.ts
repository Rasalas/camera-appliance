export type SaveState = 'pending' | 'saving' | 'saved' | 'error'

// Serialize saves and keep the latest complete draft while a request is running.
export function createDraftAutosave<T>(save: (draft: T) => Promise<unknown>, report: (state: SaveState, error?: unknown) => void, clone: (draft: T) => T, valid: (draft: T) => boolean, delay = 450) {
  let pending: { value: T; revision: number } | undefined
  let revision = 0
  let timer: ReturnType<typeof setTimeout> | undefined
  let running: Promise<void> | undefined
  let closed = false
  const notify = (state: SaveState, error?: unknown) => { if (!closed) report(state, error) }
  function cancelPending() { clearTimeout(timer); pending = undefined; revision++ }
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
        try { await save(draft.value); savedRevision = draft.revision }
        catch (err) {
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
    change(value: T) {
      cancelPending()
      if (closed || !valid(value)) return
      pending = { value: clone(value), revision }
      notify('pending')
      timer = setTimeout(() => void flush(), delay)
    },
    cancelPending,
    flush,
    close() { closed = true; return flush() }
  }
}
