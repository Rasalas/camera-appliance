import { createSnapshotSave } from '../composables/layoutSave.ts'
import type { CameraDisplay } from '../types/index.ts'

export function displaySettings(deviceID: string, display: CameraDisplay, profile: string): Record<string,string> {
  const prefix = `camera.display.${deviceID}.`
  const values = { rotation:String(display.rotation), mirror:String(display.mirror), flip:String(display.flip),
    ...(!profile ? {fit_mode:display.fit_mode} : {}),
    ...Object.fromEntries(Object.entries(display.crop).map(([key,value]) => [`crop_${key}`,String(Math.round(value))])) }
  return Object.fromEntries(Object.entries(values).map(([key,value]) => [prefix+key,value]))
}

// Each device owns a queue. Payloads include their destination keys so later
// profile switches or binding refreshes cannot redirect pending edits.
export function createDisplayPersistence(save: (settings: Record<string,string>) => Promise<unknown>, report: (error: unknown) => void, delay = 500) {
  const queues = new Map<string, ReturnType<typeof createSnapshotSave>>()
  return {
    schedule(deviceID: string, display: CameraDisplay, profile: string) {
      let queue = queues.get(deviceID)
      if (!queue) {
        queue = createSnapshotSave({delay, save: async json => {
          try { await save(JSON.parse(json)) } catch (err) { report(err); throw err }
        }})
        queues.set(deviceID,queue)
      }
      queue.schedule(displaySettings(deviceID,display,profile))
    },
    flush: async () => { await Promise.all([...queues.values()].map(queue => queue.flush())) },
    busy: () => [...queues.values()].some(queue => queue.busy()),
    async discard() { for (const queue of queues.values()) queue.cancel(); await Promise.all([...queues.values()].map(queue => queue.idle())) }
  }
}
