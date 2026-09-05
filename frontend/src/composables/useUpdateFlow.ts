import { inject, onBeforeUnmount, provide, ref, type InjectionKey } from 'vue'
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
