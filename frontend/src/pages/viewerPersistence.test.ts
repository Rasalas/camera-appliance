import test from 'node:test'
import assert from 'node:assert/strict'
import { createDisplayPersistence } from './viewerPersistence.ts'
import { gutterRatio, resizeGutterKey } from './viewerLayoutEditing.ts'
import { layoutGeometry } from './viewerMosaic.ts'
import type { CameraDisplay } from '../types/index.ts'
// Supply browser timers without a DOM.
globalThis.window = {setTimeout,clearTimeout} as unknown as Window & typeof globalThis
const display:CameraDisplay={rotation:0,mirror:false,flip:false,fit_mode:'contain',crop:{x:0,y:0,width:100,height:100}}
test('two cameras retain independent immutable snapshots within one debounce interval',async()=>{
  const saved:Record<string,string>[]=[]
  const saver=createDisplayPersistence(async value=>{saved.push(value)},()=>{})
  const draft=structuredClone(display)
  saver.schedule('device-one',draft,'')
  draft.rotation=90
  saver.schedule('device-two',draft,'wall')
  draft.crop.x=20
  await saver.flush()
  assert.equal(saved.length,2)
  assert.equal(saved[0]['camera.display.device-one.rotation'],'0')
  assert.equal(saved[1]['camera.display.device-two.rotation'],'90')
  assert.equal(saved[1]['camera.display.device-two.crop_x'],'0')
  assert.equal('camera.display.device-two.fit_mode' in saved[1],false)
})
test('same camera coalesces in-flight edits and a failed final edit remains retryable',async()=>{
  let release!:()=>void
  const saved:Record<string,string>[]=[]
  const errors:unknown[]=[]
  const saver=createDisplayPersistence(async value=>{
    saved.push(value)
    if(saved.length===1)await new Promise<void>(resolve=>{release=resolve})
    if(saved.length===2)throw Error('offline')
  },err=>errors.push(err))
  saver.schedule('stable-id',display,'')
  const pending=saver.flush()
  saver.schedule('stable-id',{...display,rotation:90},'')
  saver.schedule('stable-id',{...display,rotation:180},'')
  release()
  await assert.rejects(pending)
  assert.equal(saved.length,2)
  assert.equal(saved[1]['camera.display.stable-id.rotation'],'180')
  assert.equal(saver.busy(),true)
  await saver.flush()
  assert.equal(saver.busy(),false)
  assert.equal(errors.length,1)
})
test('keyboard gutters resize, clamp, and leave other axes alone',()=>{
  const tree={type:'split',dir:'row',ratio:0.5,a:{type:'leaf',slot:'cam1'},b:{type:'leaf',slot:'cam2'}} as const
  const gutter=layoutGeometry(tree).gutters[0]
  assert.equal(gutterRatio(gutter),0.5)
  assert.equal(resizeGutterKey(tree,gutter,'ArrowDown'),undefined)
  assert.equal((resizeGutterKey(tree,gutter,'ArrowLeft') as typeof tree).ratio,0.48)
  assert.equal((resizeGutterKey(tree,gutter,'End') as typeof tree).ratio,0.95)
})

test('pointer gutter edits commit during movement and cancel removes listeners', async()=>{
  const {createViewerLayoutEditing}=await import('./viewerLayoutEditing.ts')
  const target=new EventTarget()
  globalThis.window=target as unknown as Window & typeof globalThis
  let tree={type:'split',dir:'row',ratio:0.5,a:{type:'leaf',slot:'cam1'},b:{type:'leaf',slot:'cam2'}} as any
  let commits=0
  const editor=createViewerLayoutEditing(()=>tree,next=>{tree=next;commits++})
  const gutter=layoutGeometry(tree).gutters[0]
  const pointer=(type:string,x:number)=>Object.assign(new Event(type,{cancelable:true}),{pointerId:1,clientX:x,clientY:50}) as unknown as PointerEvent
  editor.beginResize(pointer('pointerdown',50),gutter,{getBoundingClientRect:()=>({left:0,top:0,width:100,height:100})} as HTMLElement)
  target.dispatchEvent(pointer('pointermove',75))
  assert.equal(tree.ratio,0.75)
  assert.equal(commits,1)
  target.dispatchEvent(pointer('pointercancel',75))
  target.dispatchEvent(pointer('pointermove',25))
  assert.equal(commits,1)
})
