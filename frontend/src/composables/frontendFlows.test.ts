import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'
import { createRenderer, defineComponent, h, nextTick, ssrContextKey } from 'vue'
import { createRouter, createMemoryHistory, RouterView } from 'vue-router'
const deferred = <T>() => {let resolve!:(value:T)=>void; const promise=new Promise<T>(done=>{resolve=done});return {promise,resolve}}
const tick=()=>new Promise(resolve=>setTimeout(resolve,0))

test('system settings publish before slow status and retain edits across late defaults and failed reload',async()=>{
  const server=await createServer({server:{middlewareMode:true,hmr:false,ws:false},appType:'custom',optimizeDeps:{noDiscovery:true,include:[]}})
  try {
    const {api}=await server.ssrLoadModule('/src/api/client.ts')
    const {useSystem}=await server.ssrLoadModule('/src/composables/useSystem.ts')
    const settings=deferred<Record<string,string>>(), status=deferred<unknown>()
    api.settings=()=>settings.promise; api.status=()=>status.promise
    api.authStatus=async()=>({session_hours:12,viewer_public:false,local_admin_bypass:false})
    api.events=async()=>[];api.credentialIdentities=async()=>[]
    const system=useSystem()
    const loading=system.loadAll()
    settings.resolve({'viewer.performance.mode':'quality'})
    await tick()
    assert.equal(system.sectionsLoaded.settings,true)
    assert.equal(system.sectionsLoaded.status,false)
    system.settings['viewer.performance.mode']='low'
    status.resolve({watchdog:{enabled:true,fast_interval_seconds:10},bindings:[],relays:[]})
    await loading
    assert.equal(system.settings['viewer.performance.mode'],'low')
    assert.equal(system.settingsDirty(['watchdog.enabled']),false)
    assert.equal(system.settingsDirty(['viewer.performance.mode']),true)
    api.settings=async()=>{throw Error('offline')}
    await system.loadAll()
    assert.equal(system.settings['viewer.performance.mode'],'low')
    assert.match(system.loadError.value,/offline/)
  } finally {await server.close()}
})

test('shared route guard waits for autosave on same-record param changes and requires explicit discard on failure',async()=>{
  const server=await createServer({server:{middlewareMode:true,hmr:false,ws:false},appType:'custom',optimizeDeps:{noDiscovery:true,include:[]}})
  const target=new EventTarget()
  globalThis.window=target as unknown as Window & typeof globalThis
  try {
    const {useDraftGuard,discardOpen,finishDiscard}=await server.ssrLoadModule('/src/composables/discardChanges.ts')
    let dirty=true, discard=0, save=deferred<boolean>()
    const Editor=defineComponent({setup(){useDraftGuard(()=>dirty,()=>{discard++;dirty=false},()=>save.promise);return()=>h('div')}})
    const router=createRouter({history:createMemoryHistory(),routes:[{path:'/camera/:id',component:Editor},{path:'/other',component:defineComponent({render:()=>h('div')})}]})
    const renderer=createRenderer<any,any>({
      createElement:()=>({children:[]}),createText:()=>({}),createComment:()=>({}),
      insert(){},remove(){},setText(){},setElementText(){},parentNode:()=>null,nextSibling:()=>null,patchProp(){}
    })
    await router.push('/camera/one');await router.isReady()
    const app=renderer.createApp(defineComponent({render:()=>h(RouterView)}));app.use(router);app.mount({})
    await nextTick()
    const navigate=router.push('/camera/two')
    await tick();assert.equal(router.currentRoute.value.params.id,'one')
    const unload=new Event('beforeunload',{cancelable:true});target.dispatchEvent(unload);assert.equal(unload.defaultPrevented,true)
    save.resolve(false);await tick()
    assert.equal(discardOpen.value,true)
    finishDiscard(false);await navigate
    assert.equal(router.currentRoute.value.params.id,'one')
    save=deferred<boolean>()
    const retry=router.push('/camera/two');await tick();save.resolve(true);await retry
    assert.equal(router.currentRoute.value.params.id,'two')
    save=deferred<boolean>()
    const leave=router.push('/other');await tick();save.resolve(false);await tick()
    finishDiscard(true);await leave
    assert.equal(discard,1);assert.equal(router.currentRoute.value.path,'/other')
    app.unmount()
  } finally {await server.close()}
})

test('device component starts independent requests concurrently and rejects stale camera results',async()=>{
  const server=await createServer({server:{middlewareMode:true,hmr:false,ws:false},appType:'custom',optimizeDeps:{noDiscovery:true,include:[]}})
  globalThis.window=new EventTarget() as unknown as Window & typeof globalThis
  try {
    const {api}=await server.ssrLoadModule('/src/api/client.ts')
    const {default:Page}=await server.ssrLoadModule('/src/pages/DeviceDetailsPage.vue')
    const first=deferred<any>(), calls:string[]=[]
    api.device=(id:string)=>{calls.push('device:'+id);return id==='one'?first.promise:Promise.resolve({id,label:id})}
    api.deviceCredentials=async(id:string)=>{calls.push('credentials:'+id);return {username:id,stream:'stream2'}}
    api.credentialIdentities=async()=>{calls.push('identities');return []}
    api.bindings=async()=>{calls.push('bindings');return []}
    api.settings=async()=>{calls.push('settings');return {}}
    api.relayStatus=async()=>{calls.push('relays');return []}
    const Component={...Page,render:()=>h('div')}
    const router=createRouter({history:createMemoryHistory(),routes:[{path:'/camera/:id',component:Component}]})
    const renderer=createRenderer<any,any>({createElement:()=>({}),createText:()=>({}),createComment:()=>({}),insert(){},remove(){},setText(){},setElementText(){},parentNode:()=>null,nextSibling:()=>null,patchProp(){}})
    await router.push('/camera/one');await router.isReady()
    const app=renderer.createApp(defineComponent({render:()=>h(RouterView)}));app.use(router)
    app.provide(ssrContextKey,{modules:new Set()})
    const vm=app.mount({})
    await tick()
    assert.deepEqual(calls,['device:one','credentials:one','identities','bindings','settings','relays'])
    await router.push('/camera/two');await tick()
    const page=vm.$.subTree.component!.subTree.component!
    assert.equal(page.setupState.device.id,'two')
    assert.equal(page.setupState.username,'two')
    first.resolve({id:'one',label:'Old camera'});await tick()
    assert.equal(page.setupState.device.id,'two')
    assert.equal(page.setupState.ready,true)
    app.unmount()
  } finally {await server.close()}
})

test('mask editor keyboard activation creates and selects a rectangle ready for numeric edits',async()=>{
  const server=await createServer({server:{middlewareMode:true,hmr:false,ws:false},appType:'custom',optimizeDeps:{noDiscovery:true,include:[]}})
  globalThis.requestAnimationFrame=()=>1;globalThis.cancelAnimationFrame=()=>{}
  try {
    const {api}=await server.ssrLoadModule('/src/api/client.ts')
    api.uploadImageSettings=async()=>({masks:[],timestamp:false})
    const saved:any[]=[]
    api.saveUploadImageSettings=async(id:string,draft:unknown)=>{saved.push({id,draft})}
    const {default:Editor}=await server.ssrLoadModule('/src/components/UploadImageEditor.vue')
    const renderer=createRenderer<any,any>({createElement:()=>({}),createText:()=>({}),createComment:()=>({}),insert(){},remove(){},setText(){},setElementText(){},parentNode:()=>null,nextSibling:()=>null,patchProp(){}})
    const app=renderer.createApp({...Editor,render:()=>h('div')},{deviceId:'stable-id',cameraLabel:'Kamera',src:'',capturedAt:'',crop:{enabled:true,x:0,y:0,width:100,height:100},busy:false,cropLoading:false,cropStatus:'',previewLoading:false,previewError:''})
    app.provide(ssrContextKey,{modules:new Set()})
    const vm=app.mount({})
    await tick()
    const state=vm.$.setupState
    state.arm('black',{detail:0})
    assert.equal(state.config.masks.length,1)
    assert.equal(state.selected.id,state.config.masks[0].id)
    assert.equal(state.tool,'edit')
    state.selected.x=12.5;state.selected.width=35
    state.chooseCrop(true)
    state.sourceReady=true;state.hasRendered=true
    const element={getBoundingClientRect:()=>({left:0,top:0,width:100,height:100}),focus(){},setPointerCapture(){},hasPointerCapture:()=>false}
    state.start({button:0,pointerId:1,clientX:10,clientY:10,currentTarget:element,target:{closest:()=>undefined},preventDefault(){}})
    assert.equal(state.drawing,true)
    // Starting a crop gesture must not cancel the mask edit queued just before it.
    assert.equal(await state.autosave.flush(),true)
    assert.equal(saved[0].id,'stable-id')
    assert.equal(saved[0].draft.masks[0].x,12.5)
    assert.equal(saved[0].draft.masks[0].width,35)
    state.cancel()
    state.arm('pixelate',{detail:1})
    assert.equal(state.tool,'pixelate')
    assert.equal(state.config.masks.length,1) // Pointer users still draw their rectangle.
    app.unmount()
  } finally {await server.close()}
})

test('playback accepts only current same-origin frames and transfers audio to playing HD',async()=>{
  const server=await createServer({server:{middlewareMode:true,hmr:false,ws:false},appType:'custom',optimizeDeps:{noDiscovery:true,include:[]}})
  globalThis.window={location:{origin:'http://appliance'}} as Window & typeof globalThis
  class Frame {
    messages:any[]=[]
    contentWindow={postMessage:(message:unknown)=>this.messages.push(message)}
  }
  globalThis.HTMLIFrameElement=Frame as unknown as typeof HTMLIFrameElement
  try {
    const {useViewerFrames}=await server.ssrLoadModule('/src/composables/viewerFrames.ts')
    const frames=useViewerFrames({enabled:()=>true,alias:()=>'cam1'})
    const sub=new Frame(), hd=new Frame(), replaced=new Frame()
    frames.setFrameRef('cam1',sub);frames.setHDFrameRef('cam1',hd)
    const message=(source:unknown,origin='http://appliance')=>({origin,source,data:{type:'camera-stream-ready',src:'cam1_stream1'}})
    frames.handleEmbedMessage(message(hd.contentWindow,'http://elsewhere'))
    frames.handleEmbedMessage(message(replaced.contentWindow))
    assert.equal(frames.hdStatus.value.cam1,undefined)
    frames.handleEmbedMessage(message(hd.contentWindow))
    assert.equal(frames.hdStatus.value.cam1,'playing')
    assert.equal(sub.messages.at(-1).muted,true)
    assert.equal(hd.messages.at(-1).muted,false)
    frames.setHDFrameRef('cam1',null);frames.syncAudioState()
    assert.equal(sub.messages.at(-1).muted,false)
    frames.dispose()
  } finally {await server.close()}
})
