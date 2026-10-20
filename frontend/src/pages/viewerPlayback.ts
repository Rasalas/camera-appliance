// Pure playback decisions for the viewer: which iframes run and how embed
// states and offline slots are presented.
import type { ViewerPerformanceMode, ViewerSlotState } from '../types'

export type FrameStatus = 'connecting' | 'playing' | 'stalled' | 'error'
export type PlayerInput = { alias: string; playable: boolean; hasHD: boolean }
export type PlanContext = {
  mode: ViewerPerformanceMode
  primary: string
  spotlight: string
  released: ReadonlySet<string>
  paused: boolean
  hdStatus: Readonly<Record<string, FrameStatus>>
}
export type PlayerPlan = { sub: boolean; hd: boolean; waiting: boolean }

export const HD_SUFFIX = '_stream1'
export const STAGGER_MS = 1500

const NONE: PlayerPlan = { sub: false, hd: false, waiting: false }

export function planPlayer(slot: PlayerInput, ctx: PlanContext): PlayerPlan {
  if (!slot.playable || ctx.paused) return NONE
  if (ctx.spotlight) {
    if (ctx.spotlight !== slot.alias) return NONE
    // The sub stream stays underneath until HD plays, so there is no black flash.
    return { sub: !(slot.hasHD && ctx.hdStatus[slot.alias] === 'playing'), hd: slot.hasHD, waiting: false }
  }
  if (ctx.mode === 'low') return { sub: slot.alias === ctx.primary, hd: false, waiting: false }
  if (ctx.mode === 'balanced' && slot.alias !== ctx.primary && !ctx.released.has(slot.alias)) {
    return { sub: false, hd: false, waiting: true }
  }
  return { sub: true, hd: false, waiting: false }
}

const STATES: readonly FrameStatus[] = ['connecting', 'playing', 'stalled', 'error']

export function parseEmbedMessage(data: unknown): { alias: string; hd: boolean; state: FrameStatus } | undefined {
  if (!data || typeof data !== 'object') return undefined
  const message = data as { type?: unknown; src?: unknown; state?: unknown }
  if (typeof message.src !== 'string' || !message.src) return undefined
  let state: FrameStatus
  if (message.type === 'camera-stream-ready') state = 'playing'
  else if (message.type === 'camera-stream-state' && STATES.includes(message.state as FrameStatus)) state = message.state as FrameStatus
  else return undefined
  const hd = message.src.endsWith(HD_SUFFIX)
  return { alias: hd ? message.src.slice(0, -HD_SUFFIX.length) : message.src, hd, state }
}

export function offlineMessage(state: ViewerSlotState): string {
  if (state === 'credentials_failed') return 'Anmeldung an der Kamera fehlgeschlagen'
  return 'Kamera nicht erreichbar – verbindet automatisch neu'
}
