import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createDraftAutosave } from './draftAutosave.ts'
import { cloneImageSettings, frameRect, timestampText, validImageSettings } from './uploadImagePreview.ts'
import type { UploadImageSettings } from '../types/index.ts'

test('keeps device-local capture time and rounds mask edges outwards', () => {
  assert.equal(timestampText('2026-09-05T12:34:56+02:00'), '05.09.2026 12:34:56')
  assert.deepEqual(frameRect({x:10.1,y:20.2,width:30.3,height:40.4},320,240),{x:32,y:48,width:98,height:98})
  assert.throws(()=>timestampText(''))
  assert.equal(validImageSettings({masks:[{id:'a',mode:'black',x:99,y:0,width:5,height:10}],timestamp:false}),false)
})

test('privacy autosave retains complete mask sets during slow and failed writes', async () => {
  const draft:UploadImageSettings={masks:[{id:'a',mode:'black',x:10,y:10,width:20,height:20}],timestamp:false}
  const saved:UploadImageSettings[]=[]
  let release!:()=>void
  const states:string[]=[]
  const autosave=createDraftAutosave(async(value:UploadImageSettings)=>{
    saved.push(value)
    if(saved.length===1)await new Promise<void>(resolve=>{release=resolve})
    if(saved.length===2)throw new Error('Offline')
  },state=>states.push(state),cloneImageSettings,validImageSettings)
  autosave.change(draft)
  const pending=autosave.flush()
  draft.masks[0].x=30;draft.timestamp=true
  autosave.change(draft)
  release();await pending
  assert.equal(saved[0].masks[0].x,10)
  assert.equal(saved[0].timestamp,false)
  assert.equal(states.at(-1),'error')
  await autosave.flush()
  assert.deepEqual(saved.at(-1),draft)
  assert.equal(states.at(-1),'saved')
  await autosave.close()
})

test('navigation waits for the newest privacy edit and blocks on failure until retry', async () => {
  let release!: () => void
  let calls = 0
  const draft:UploadImageSettings={masks:[],timestamp:false}
  const autosave=createDraftAutosave(async () => {
    if (++calls === 1) await new Promise<void>(resolve => {release=resolve})
    else if (calls === 2) throw new Error('Offline')
  },()=>{},cloneImageSettings,validImageSettings)
  autosave.change(draft)
  const initial=autosave.flush()
  autosave.change({...draft,timestamp:true})
  const navigation=autosave.flush()
  release()
  assert.equal(await initial,false)
  assert.equal(await navigation,false)
  assert.equal(autosave.busy(),true)
  assert.equal(await autosave.flush(),true)
  assert.equal(autosave.busy(),false)
})

test('failed close reports error and keeps the draft open for retry', async () => {
  let fail=true
  const states:string[]=[]
  const autosave=createDraftAutosave(async () => {if(fail)throw new Error('Offline')},state=>states.push(state),cloneImageSettings,validImageSettings)
  autosave.change({masks:[],timestamp:true})
  await assert.rejects(autosave.close())
  assert.equal(states.at(-1),'error')
  assert.equal(autosave.busy(),true)
  fail=false
  await autosave.close()
  assert.equal(states.at(-1),'saved')
})

test('40MP preview work is bounded, source pixels cached, and timestamp uses original coordinates', async () => {
  const {paintImagePreview,previewDimensions}=await import('./uploadImagePreview.ts')
  assert.deepEqual(previewDimensions(8000,5000),{width:960,height:600})
  let reads=0, draws=0, resets=0
  const rectangles:number[][]=[]
  const context={
    drawImage(...args:unknown[]){if(args.length===5){draws++;assert.deepEqual(args.slice(1),[0,0,960,600])}},
    getImageData(_x:number,_y:number,w:number,h:number){reads++;assert.ok(w*h<=960*960);return {data:new Uint8ClampedArray(w*h*4).fill(80)}},
    fillRect(...rect:number[]){rectangles.push(rect)}, fillStyle:'',save(){},restore(){},scale(){}
  }
  const makeCanvas=()=>{
    let w=0,h=0
    return {get width(){return w},set width(value:number){w=value;resets++},get height(){return h},set height(value:number){h=value;resets++},getContext:()=>context} as unknown as HTMLCanvasElement
  }
  globalThis.document={createElement:makeCanvas} as unknown as Document
  const source={naturalWidth:8000,naturalHeight:5000} as HTMLImageElement
  const canvas=makeCanvas()
  const settings:UploadImageSettings={masks:[{id:'privacy',mode:'black',x:10,y:20,width:30,height:40}],timestamp:false}
  const crop={enabled:false,x:0,y:0,width:100,height:100}
  paintImagePreview(canvas,source,settings,crop,'')
  assert.deepEqual(rectangles.at(-1),[96,120,288,240])
  const firstResets=resets
  settings.masks[0].mode='pixelate'
  paintImagePreview(canvas,source,settings,crop,'')
  assert.equal(reads,1);assert.equal(draws,1);assert.equal(resets,firstResets)
  // 160x50 original pixels, only 19x6 preview pixels. Validation uses the original.
  paintImagePreview(canvas,source,{masks:[],timestamp:true},{enabled:true,x:0,y:0,width:2,height:1},'2026-09-05T12:34:56+02:00')
})
