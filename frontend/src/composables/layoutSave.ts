// Snapshot at schedule time, serialize requests, and retain failed work for retry.
type Timers = { schedule: (callback: () => void, ms: number) => number; cancel: (id: number) => void }
export function createSnapshotSave(options: { delay: number; save: (snapshot: string) => Promise<void> } & Partial<Timers>) {
  const schedule = options.schedule ?? ((callback, ms) => window.setTimeout(callback, ms))
  const cancel = options.cancel ?? ((id) => window.clearTimeout(id))
  let timer = 0
  let snapshot: string | undefined
  let inFlight: Promise<void> | undefined
  let revision = 0
  function flush(): Promise<void> {
    cancel(timer); timer = 0
    if (inFlight) return inFlight
    if (snapshot === undefined) return Promise.resolve()
    inFlight = (async () => {
      while (snapshot !== undefined) {
        cancel(timer); timer = 0
        const value = snapshot, current = revision
        snapshot = undefined
        try { await options.save(value) }
        catch (err) {
          if (current === revision && snapshot === undefined) snapshot = value
          throw err
        }
      }
    })().finally(() => { inFlight = undefined })
    return inFlight
  }
  return {
    schedule(value: unknown) {
      snapshot = JSON.stringify(value); revision++
      cancel(timer)
      timer = schedule(() => { void flush().catch(() => undefined) }, options.delay)
    },
    flush,
    busy: () => snapshot !== undefined || inFlight !== undefined,
    idle: () => (inFlight ?? Promise.resolve()).catch(() => undefined),
    cancel() { cancel(timer); timer = 0; snapshot = undefined; revision++ }
  }
}
