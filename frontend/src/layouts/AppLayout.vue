<template>
  <div class="shell">
    <aside class="rail">
      <div class="brand">
        <span class="mark" />
        <span class="name">Watch<em>deck</em></span>
      </div>

      <nav class="nav">
        <RouterLink to="/">Kameras<span class="nav-key">1</span></RouterLink>
        <RouterLink to="/einrichtung">Einrichtung<span class="nav-key">2</span></RouterLink>
        <RouterLink to="/uebersicht">Status<span class="nav-key">3</span></RouterLink>
        <RouterLink to="/system">System<span class="nav-key">4</span></RouterLink>
      </nav>

      <div class="rail-foot">
        <div class="row"><span>Stand</span><b>{{ clock }}</b></div>
        <div class="row"><span>Bind</span><b>127.0.0.1:8091</b></div>
        <div class="row"><span>Build</span><b>local</b></div>
      </div>
    </aside>

    <main class="canvas">
      <RouterView v-slot="{ Component, route }">
        <div :key="route.fullPath" class="route-fade">
          <component :is="Component" />
        </div>
      </RouterView>
    </main>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const clock = ref('')

function tick() {
  clock.value = new Date().toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
let timer = 0
let onKey: ((e: KeyboardEvent) => void) | undefined

onMounted(() => {
  tick()
  timer = window.setInterval(tick, 1000)
  onKey = (e: KeyboardEvent) => {
    if (e.target instanceof HTMLElement && ['INPUT', 'TEXTAREA', 'SELECT'].includes(e.target.tagName)) return
    if (e.key === '1') router.push('/')
    if (e.key === '2') router.push('/einrichtung')
    if (e.key === '3') router.push('/uebersicht')
    if (e.key === '4') router.push('/system')
  }
  window.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => {
  window.clearInterval(timer)
  if (onKey) window.removeEventListener('keydown', onKey)
})
</script>

<style scoped>
.route-fade {
  display: flex;
  flex-direction: column;
  gap: var(--gutter);
  animation: route-in .18s ease-out both;
}
@keyframes route-in {
  from { opacity: 0; }
  to   { opacity: 1; }
}
</style>
