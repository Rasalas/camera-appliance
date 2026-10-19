// Debounced save that serializes the value when scheduled, so a refresh that
// replaces the live object cannot change what gets written.
type Timers = { schedule: (callback: () => void, ms: number) => number; cancel: (id: number) => void }

export function createSnapshotSave(options: { delay: number; save: (snapshot: string) => Promise<void> } & Partial<Timers>) {
  const schedule = options.schedule ?? ((callback, ms) => window.setTimeout(callback, ms))
  const cancel = options.cancel ?? ((id) => window.clearTimeout(id))
  let timer = 0
  let snapshot: string | undefined
  let inFlight: Promise<void> | undefined

  function flush(): Promise<void> {
    cancel(timer)
    timer = 0
    const value = snapshot
    snapshot = undefined
    if (value === undefined) return inFlight ?? Promise.resolve()
    const run = inFlight ? inFlight.catch(() => undefined).then(() => options.save(value)) : options.save(value)
    const tracked = run.finally(() => { if (inFlight === tracked) inFlight = undefined })
    inFlight = tracked
    return tracked
  }

  return {
    schedule(value: unknown) {
      snapshot = JSON.stringify(value)
      cancel(timer)
      timer = schedule(() => { void flush().catch(() => undefined) }, options.delay)
    },
    flush,
    busy: () => snapshot !== undefined || inFlight !== undefined,
    idle: () => (inFlight ?? Promise.resolve()).catch(() => undefined),
    cancel() {
      cancel(timer)
      timer = 0
      snapshot = undefined
    }
  }
}
