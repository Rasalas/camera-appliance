export type RestartOutcome = { kind: 'ready' } | { kind: 'failed'; message: string } | { kind: 'timeout' }

type Deps = {
  trigger: () => Promise<unknown>
  probe: () => Promise<boolean>
  sleep: (ms: number) => Promise<void>
  now: () => number
  onProgress?: (elapsedSeconds: number) => void
  graceMs?: number
  timeoutMs?: number
  intervalMs?: number
}

export const RESTART_TIMEOUT_MS = 180_000

// Ready means the station answered after it was seen down, or the grace period
// passed after the restart request returned. An HTTP error means nothing restarted.
export async function restartStation(deps: Deps): Promise<RestartOutcome> {
  const { graceMs = 8_000, timeoutMs = RESTART_TIMEOUT_MS, intervalMs = 1_500 } = deps
  const started = deps.now()
  let wentDown = false, settled = false, failure: string | undefined
  deps.trigger().then(() => { settled = true }, (err: unknown) => {
    settled = true
    if ((err as { kind?: string })?.kind === 'http') failure = err instanceof Error ? err.message : 'Neustart abgelehnt.'
    else wentDown = true
  })
  while (deps.now() - started < timeoutMs) {
    await deps.sleep(intervalMs)
    deps.onProgress?.(Math.round((deps.now() - started) / 1000))
    if (failure !== undefined) return { kind: 'failed', message: failure }
    if (!await deps.probe()) { wentDown = true; continue }
    if (wentDown || (settled && deps.now() - started >= graceMs)) return { kind: 'ready' }
  }
  return { kind: 'timeout' }
}
