import type { UpdateFlowStatus } from '../types/index.ts'

type UpdateAPI = {
  getUpdateStatus(): Promise<UpdateFlowStatus>
  checkForUpdates(): Promise<UpdateFlowStatus>
  downloadUpdate(): Promise<UpdateFlowStatus>
  installUpdate(): Promise<unknown>
}
type Hooks = {
  publish(status: UpdateFlowStatus): void
  busy(value: boolean): void
  reload(): void
  schedule(callback: () => void): void
  cancel(): void
}

// Keep restart recovery independent of the popover and touch gestures.
export function createUpdateClient(api: UpdateAPI, hooks: Hooks) {
  let status: UpdateFlowStatus | undefined
  let loadedVersion: string | undefined
  let reloaded = false, disposed = false, requesting = false, revision = 0
  let reading: Promise<void> | undefined
  const working = () => status?.phase === 'downloading' || status?.phase === 'installing'
  function adopt(next: UpdateFlowStatus) {
    if (disposed) return
    loadedVersion ??= next.current_version
    status = next
    hooks.publish(next)
    const job = next.job
    const verified = job?.phase === 'complete' ? job.result?.new_version?.version
      : job?.phase === 'failed' && job.result?.rollback_applied ? job.result.old_version?.version : undefined
    // Reload only after the durable job has verified the effective release.
    // A fresh page records that version and cannot enter a reload loop.
    if (!reloaded && verified === next.current_version && next.current_version !== loadedVersion) {
      reloaded = true
      hooks.reload()
    }
  }
  function poll() {
    hooks.cancel()
    if (!disposed && !requesting && !reloaded && working()) hooks.schedule(() => void refresh())
  }
  function refresh(): Promise<void> {
    if (reading) return reading
    if (disposed) return Promise.resolve()
    const started = revision
    reading = (async () => {
      try {
        const next = await api.getUpdateStatus()
        if (started === revision) adopt(next)
      } catch { /* A restart interrupts HTTP; retain the job phase and retry. */ }
    })().finally(() => { reading = undefined; poll() })
    return reading
  }
  async function perform(action: 'check' | 'download' | 'install', submit = () => api.installUpdate()) {
    if (disposed || requesting || working()) return
    requesting = true; revision++; hooks.cancel(); hooks.busy(true)
    if (action === 'install') adopt({ ...(status ?? { current_version: '' }), phase: 'installing', error: '' })
    try {
      if (action === 'install') {
        await submit()
        await refresh()
      } else adopt(await (action === 'check' ? api.checkForUpdates() : api.downloadUpdate()))
    } catch (error) {
      if (action === 'install' || action === 'check') {
        // An HTTP failure does not tell us whether the server accepted a job.
        // Read its durable status; never submit the installation again here.
        await refresh()
      } else adopt({ ...(status ?? { current_version: '' }), phase: 'failed', error: error instanceof Error ? error.message : 'Download fehlgeschlagen.' })
    } finally {
      requesting = false
      if (!disposed) hooks.busy(false)
      poll()
    }
  }
  return { refresh, check: () => perform('check'), download: () => perform('download'), install: (submit?: () => Promise<unknown>) => perform('install', submit), close() { disposed = true; hooks.cancel() } }
}
