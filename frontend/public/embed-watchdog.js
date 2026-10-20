// Detects frozen streams: live video must keep advancing currentTime (MSE/MP4)
// or replacing its poster (go2rtc's MJPEG fallback).
export const SAMPLE_MS = 1000
export const STALL_MS = 20000
export const MAX_STALL_MS = 60000

export function stallTimeout(attempt) {
  return Math.min(MAX_STALL_MS, STALL_MS * 2 ** attempt)
}

export function retryDelay(attempt) {
  return Math.min(60000, 2000 * 2 ** attempt)
}

export function createStallWatch(now) {
  let lastProgress = now
  let last
  let playing = false
  let attempt = 0

  return {
    sample(now, signal) {
      const progressed = !!last && (signal.currentTime !== last.currentTime || signal.poster !== last.poster)
      last = { currentTime: signal.currentTime, poster: signal.poster }
      if (progressed) {
        lastProgress = now
        playing = true
        attempt = 0
      }
      const limit = playing ? STALL_MS : stallTimeout(attempt)
      const restart = now - lastProgress >= limit
      let state = playing ? 'playing' : 'connecting'
      if (signal.error) state = 'error'
      else if (restart) state = 'stalled'
      return { state, restart }
    },
    restarted(now) {
      lastProgress = now
      last = undefined
      playing = false
      attempt = Math.min(attempt + 1, 8)
    }
  }
}
