export type SaveState = 'pending' | 'saving' | 'saved' | 'error'

// Serialize saves and keep the latest complete draft while a request is running.
export function createDraftAutosave<T>(save: (draft: T) => Promise<unknown>, report: (state: SaveState, error?: unknown) => void, clone: (draft: T) => T, valid: (draft: T) => boolean, delay = 450) {
  let pending: { value: T; revision: number } | undefined
  let revision = 0
  let timer: ReturnType<typeof setTimeout> | undefined
  let running: Promise<boolean> | undefined
  let closed = false
  let invalid = false
  const notify = (state: SaveState, error?: unknown) => { if (!closed) report(state, error) }
  function cancelPending() { clearTimeout(timer); pending = undefined; revision++ }
  function flush(): Promise<boolean> {
    clearTimeout(timer)
    if (running) return running
    if (!pending) return Promise.resolve(!invalid)
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
            return false
          }
        }
      }
      if (savedRevision === revision) notify('saved')
      return !invalid
    })().finally(() => { running = undefined })
    return running
  }
  return {
    change(value: T) {
      if (closed) return
      cancelPending()
      invalid = !valid(value)
      if (invalid) return
      pending = { value: clone(value), revision }
      notify('pending')
      timer = setTimeout(() => void flush(), delay)
    },
    cancelPending,
    flush,
    busy: () => invalid || !!pending || !!running,
    async discard() { cancelPending(); invalid = false; await running },
    async close() {
      if (!await flush()) throw new Error('Änderungen konnten nicht gespeichert werden.')
      closed = true
    }
  }
}
