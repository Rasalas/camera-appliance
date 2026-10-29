import { setRatioAtPath } from './viewerMosaic.ts'
import type { MosaicNode, GutterRect } from './viewerMosaic.ts'
export function gutterRatio(gutter: GutterRect) {
  return gutter.dir === 'row' ? (gutter.line.x-gutter.rect.x)/gutter.rect.w : (gutter.line.y-gutter.rect.y)/gutter.rect.h
}
export function resizeGutterKey(tree: MosaicNode, gutter: GutterRect, key: string, large = false): MosaicNode | undefined {
  const keys = gutter.dir === 'row' ? ['ArrowLeft','ArrowRight'] : ['ArrowUp','ArrowDown']
  const step = large ? 0.1 : 0.02
  const ratio = key === 'Home' ? 0.05 : key === 'End' ? 0.95 : keys.includes(key) ? gutterRatio(gutter)+(key === keys[0] ? -step : step) : undefined
  return ratio === undefined ? undefined : setRatioAtPath(tree,gutter.path,ratio)
}

// Pointer and keyboard edits share one commit path, including persistence.
export function createViewerLayoutEditing(tree: () => MosaicNode | undefined, commit: (value: MosaicNode) => void) {
  let stop: (() => void) | undefined
  return {
    resizeKey(gutter: GutterRect, key: string, large = false) {
      const current=tree()
      const next=current && resizeGutterKey(current,gutter,key,large)
      if (!next) return false
      commit(next);return true
    },
    beginResize(event: PointerEvent, gutter: GutterRect, element: HTMLElement) {
      if (!tree()) return
      stop?.();event.preventDefault()
      const box=element.getBoundingClientRect()
      const move=(next: PointerEvent) => {
        if (next.pointerId !== event.pointerId) return
        const current=tree()
        if (!current || !box.width || !box.height) return
        const ratio=gutter.dir === 'row'
          ? (((next.clientX-box.left)/box.width)*100-gutter.rect.x)/gutter.rect.w
          : (((next.clientY-box.top)/box.height)*100-gutter.rect.y)/gutter.rect.h
        commit(setRatioAtPath(current,gutter.path,ratio));next.preventDefault()
      }
      const finish=(next: PointerEvent) => {if(next.pointerId===event.pointerId)stop?.()}
      stop=()=>{
        window.removeEventListener('pointermove',move)
        window.removeEventListener('pointerup',finish)
        window.removeEventListener('pointercancel',finish)
        stop=undefined
      }
      window.addEventListener('pointermove',move)
      window.addEventListener('pointerup',finish)
      window.addEventListener('pointercancel',finish)
    },
    stop() {stop?.()}
  }
}
