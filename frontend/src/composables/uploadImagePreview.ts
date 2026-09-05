import fontData from '../../../camera-manager/internal/snapshotupload/stamp-font.json' with { type: 'json' }
import type { UploadCrop, UploadImageSettings } from '../types/index.ts'
import { validUploadCrop } from './uploadCropDraft.ts'

export function validImageSettings(value: UploadImageSettings) {
  return Array.isArray(value.masks) && value.masks.length <= 16 && typeof value.timestamp === 'boolean'
    && new Set(value.masks.map(m => m.id)).size === value.masks.length
    && value.masks.every(m => /^[a-zA-Z0-9_-]{1,80}$/.test(m.id) && ['black', 'pixelate'].includes(m.mode) && validUploadCrop({ ...m, enabled: true }))
}
export function cloneImageSettings(value: UploadImageSettings): UploadImageSettings {
  return { masks: value.masks.map(m => ({ ...m })), timestamp: value.timestamp }
}
export function frameRect(rect: Pick<UploadCrop, 'x'|'y'|'width'|'height'>, width: number, height: number) {
  const x = Math.floor(rect.x * width / 100), y = Math.floor(rect.y * height / 100)
  return { x, y, width: Math.min(width, Math.ceil((rect.x + rect.width) * width / 100)) - x, height: Math.min(height, Math.ceil((rect.y + rect.height) * height / 100)) - y }
}
export function timestampText(capturedAt: string) {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}/.test(capturedAt)) throw new Error('Aufnahmezeit fehlt. Bitte die Vorschau erneut laden.')
  // Keep the device's UTC offset, rather than converting to the browser's zone.
  return `${capturedAt.slice(8,10)}.${capturedAt.slice(5,7)}.${capturedAt.slice(0,4)} ${capturedAt.slice(11,19)}`
}

export function paintImagePreview(canvas: HTMLCanvasElement, source: HTMLImageElement, settings: UploadImageSettings, crop: UploadCrop, capturedAt: string) {
  if (!validImageSettings(settings) || !validUploadCrop(crop)) throw new Error('Privatbereiche oder Bildausschnitt ungültig. Upload gesperrt.')
  const width = source.naturalWidth, height = source.naturalHeight
  if (!width || !height || width * height > 40_000_000) throw new Error('Vorschaubild ist ungültig oder zu groß.')
  canvas.width = width; canvas.height = height
  const ctx = canvas.getContext('2d', { willReadFrequently: true })
  if (!ctx) throw new Error('Bildvorschau kann nicht bearbeitet werden. Upload gesperrt.')
  ctx.drawImage(source, 0, 0)
  const pixels = settings.masks.some(m => m.mode === 'pixelate') ? ctx.getImageData(0, 0, width, height).data : undefined
  for (const mask of settings.masks) {
    if (mask.mode !== 'pixelate' || !pixels) continue
    const r = frameRect(mask, width, height), block = Math.max(16, Math.ceil(Math.max(r.width,r.height)/8))
    for (let y=r.y; y<r.y+r.height; y+=block) for (let x=r.x; x<r.x+r.width; x+=block) {
      const right = Math.min(x+block,r.x+r.width), bottom = Math.min(y+block,r.y+r.height)
      let red=0, green=0, blue=0
      for (let py=y; py<bottom; py++) for (let px=x; px<right; px++) { const i=(py*width+px)*4; red+=pixels[i]; green+=pixels[i+1]; blue+=pixels[i+2] }
      const n=(right-x)*(bottom-y)
      ctx.fillStyle=`rgb(${Math.floor(red/n)},${Math.floor(green/n)},${Math.floor(blue/n)})`
      ctx.fillRect(x,y,right-x,bottom-y)
    }
  }
  ctx.fillStyle='#000'
  for (const mask of settings.masks) { if (mask.mode==='black') { const r=frameRect(mask,width,height); ctx.fillRect(r.x,r.y,r.width,r.height) } }
  if (!settings.timestamp) return
  const text=timestampText(capturedAt)
  const r=crop.enabled ? frameRect(crop,width,height) : {x:0,y:0,width,height}
  if(r.width<127 || r.height<21) throw new Error('Für Datum und Uhrzeit muss das fertige Bild mindestens 127 × 21 Pixel groß sein.')
  const scale=Math.min(Math.max(2,Math.floor(r.width/600)),Math.floor(r.width/127),Math.floor(r.height/21)), padding=3*scale, margin=4*scale
  const boxWidth=(text.length*6-1)*scale+2*padding, boxHeight=7*scale+2*padding
  if(r.width<boxWidth+2*margin || r.height<boxHeight+2*margin) throw new Error('Für Datum und Uhrzeit muss das fertige Bild mindestens 127 × 21 Pixel groß sein.')
  const left=r.x+r.width-margin-boxWidth, top=r.y+r.height-margin-boxHeight
  ctx.fillRect(left,top,boxWidth,boxHeight)
  ctx.fillStyle='#fff'
  const font: Record<string,number[]> = fontData
  for (let i=0;i<text.length;i++) font[text[i]].forEach((row,y)=>{for(let x=0;x<5;x++) if(row&(1<<(4-x))) ctx.fillRect(left+padding+(i*6+x)*scale,top+padding+y*scale,scale,scale)})
}
