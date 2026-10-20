// Kiosk mode survives in-app navigation: ?kiosk=1 is remembered per browser
// profile until ?kiosk=0 is opened.
export const KIOSK_STORAGE_KEY = 'camera-appliance.viewer.kiosk'
export const ADMIN_IDLE_MS = 10 * 60 * 1000

type KioskStorage = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>

export function resolveKioskFlag(search: string, storage?: KioskStorage): boolean {
  const param = new URLSearchParams(search).get('kiosk')
  try {
    if (param === '1') storage?.setItem(KIOSK_STORAGE_KEY, '1')
    else if (param === '0') storage?.removeItem(KIOSK_STORAGE_KEY)
  } catch {
    // Storage may be disabled; the URL still applies to this visit.
  }
  if (param === '1' || param === '0') return param === '1'
  return storedKioskFlag(storage)
}

export function storedKioskFlag(storage?: KioskStorage): boolean {
  try {
    return storage?.getItem(KIOSK_STORAGE_KEY) === '1'
  } catch {
    return false
  }
}

export function browserStorage(): KioskStorage | undefined {
  try {
    return window.localStorage
  } catch {
    return undefined
  }
}

export function shouldReturnToViewer(input: { kiosk: boolean; onViewer: boolean; idleMs: number; unsaved: boolean }) {
  return input.kiosk && !input.onViewer && !input.unsaved && input.idleMs >= ADMIN_IDLE_MS
}

// The first version seen is the one this page was loaded with.
export function createVersionWatch() {
  let initial = ''
  return {
    changed(version: string | undefined): boolean {
      if (!version) return false
      if (!initial) initial = version
      return version !== initial
    }
  }
}
