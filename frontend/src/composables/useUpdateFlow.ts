import { inject, onBeforeUnmount, provide, ref, watch, type Ref, type InjectionKey } from 'vue'
import { api } from '../api/client'
import type { UpdateFlowStatus } from '../types'
import { createUpdateClient } from './updateClient'

function createFlow() {
  const status = ref<UpdateFlowStatus>(), busy = ref(false)
  let timer = 0
  const client = createUpdateClient(api, {
    publish: value => { status.value = value }, busy: value => { busy.value = value },
    reload: () => window.location.reload(),
    schedule: callback => { timer = window.setTimeout(callback, 1000) },
    cancel: () => window.clearTimeout(timer)
  })
  return { status, busy, client }
}
const key: InjectionKey<ReturnType<typeof createFlow>> = Symbol('update-flow')
export function provideUpdateFlow() {
  const flow = createFlow()
  provide(key, flow)
  onBeforeUnmount(() => flow.client.close())
  return flow
}
export function useUpdateFlow() {
  const flow = inject(key)
  if (!flow) throw new Error('Update flow requires the application layout')
  return flow
}

// Keep update recovery and periodic checks alive independently of the About page.
export function useUpdateMonitoring(flow: ReturnType<typeof createFlow>, enabled: Ref<boolean>) {
  let interval = 0
  watch(enabled, active => {
    window.clearInterval(interval)
    if (!active) return
    void flow.client.refresh().then(() => {
      if (enabled.value && flow.status.value?.phase === 'idle') void flow.client.check()
    })
    interval = window.setInterval(() => {
      const phase = flow.status.value?.phase
      if (enabled.value && !flow.busy.value && phase !== 'ready' && phase !== 'downloading' && phase !== 'installing') void flow.client.check()
    }, 6 * 60 * 60 * 1000)
  }, { immediate: true })
  onBeforeUnmount(() => window.clearInterval(interval))
}
