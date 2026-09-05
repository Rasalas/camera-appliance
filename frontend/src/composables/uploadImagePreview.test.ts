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
