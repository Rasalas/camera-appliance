import { ref } from 'vue'
import { parseEmbedMessage } from '../pages/viewerPlayback'
import type { FrameStatus } from '../pages/viewerPlayback'

// Own iframe identity, trusted embed messages, and single-stream audio handoff.
export function useViewerFrames(audio: { enabled: () => boolean; alias: () => string }) {
  const subStatus = ref<Record<string, FrameStatus>>({}), hdStatus = ref<Record<string, FrameStatus>>({})
  const sub: Record<string, HTMLIFrameElement> = {}, hd: Record<string, HTMLIFrameElement> = {}
  let timer: ReturnType<typeof setTimeout> | undefined
  let disposed = false
  function syncAudioState() {
    if (disposed) return
    for (const [frames,isHD] of [[sub,false],[hd,true]] as const) {
      for (const [alias,frame] of Object.entries(frames)) {
        const hdPlaying = !!hd[alias] && hdStatus.value[alias] === 'playing'
        const audible = audio.enabled() && audio.alias() === alias && isHD === hdPlaying
        frame.contentWindow?.postMessage({type:'camera-audio',muted:!audible},window.location.origin)
      }
    }
  }
  function track(alias: string, element: unknown, high: boolean) {
    const frames = high ? hd : sub, status = high ? hdStatus : subStatus
    const next = element instanceof HTMLIFrameElement ? element : undefined
    if (frames[alias] === next) return
    if (next) frames[alias] = next
    else delete frames[alias]
    const {[alias]:_old,...rest} = status.value
    status.value = rest
    clearTimeout(timer)
    timer = setTimeout(syncAudioState,0)
  }
  return {
    subStatus, hdStatus, syncAudioState,
    setFrameRef: (alias: string, el: unknown) => track(alias,el,false),
    setHDFrameRef: (alias: string, el: unknown) => track(alias,el,true),
    handleEmbedMessage(event: MessageEvent) {
      if (disposed || event.origin !== window.location.origin) return
      const message = parseEmbedMessage(event.data)
      if (!message) return
      const frame = (message.hd ? hd : sub)[message.alias]
      if (!frame || event.source !== frame.contentWindow) return
      const status = message.hd ? hdStatus : subStatus
      if (status.value[message.alias] === message.state) return
      status.value = {...status.value,[message.alias]:message.state}
      syncAudioState()
    },
    dispose() { disposed = true; clearTimeout(timer) }
  }
}
