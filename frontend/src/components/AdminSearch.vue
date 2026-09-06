<template>
  <div class="admin-search">
    <button ref="trigger" class="search-trigger btn" aria-label="Verwaltung durchsuchen" aria-haspopup="dialog" :aria-expanded="open" @click="toggle"><AppIcon name="search" /><span>Suchen</span><kbd>⌘ K</kbd></button>
    <section v-if="open" ref="popup" class="search-popover" role="dialog" aria-label="Verwaltung durchsuchen" @keydown.esc.stop.prevent="close" @keydown="trap">
      <div class="search-input-row"><input ref="input" v-model="query" type="search" placeholder="Name, IP oder Benutzername" aria-label="Suchbegriff" autocomplete="off" role="combobox" aria-autocomplete="list" aria-controls="resource-results" aria-expanded="true" :aria-activedescendant="results[active] ? `search-${active}` : undefined" @keydown.down.prevent="move(1)" @keydown.up.prevent="move(-1)" @keydown.enter.prevent="choose" /><button class="btn icon ghost" aria-label="Suche schließen" @click="close">×</button></div>
      <fieldset v-if="currentKind" class="search-scope"><legend class="sr-only">Suchbereich</legend><label><input v-model="scope" type="radio" value="all" />Alle Bereiche</label><label><input v-model="scope" type="radio" :value="currentKind" />Nur {{ currentKind }}</label></fieldset>
      <p v-if="loading" role="status">Ressourcen werden geladen…</p>
      <p v-else-if="error" role="alert">{{ error }} <button class="btn" @click="load">Erneut laden</button></p>
      <p v-else-if="!results.length" role="status">{{ query ? 'Keine passenden Ressourcen.' : 'Noch keine Ressourcen vorhanden.' }}</p>
      <div v-else id="resource-results" class="search-results" role="listbox" aria-label="Suchergebnisse">
        <template v-for="(entry, index) in results" :key="entry.kind + entry.id">
          <h3 v-if="index === 0 || results[index - 1]?.kind !== entry.kind">{{ entry.kind }}</h3>
          <a :id="`search-${index}`" :href="entry.href" role="option" :aria-selected="active === index" @mousemove="active = index" @click.prevent="navigate(entry.href)"><span>{{ entry.title }}</span><small>{{ entry.detail }}</small></a>
        </template>
      </div>
    </section>
  </div>
</template>
<script setup lang="ts">
import AppIcon from './AppIcon.vue'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import { isCameraResource } from '../composables/cameraResources'
import { searchResources, type ResourceKind, type SearchEntry } from '../composables/adminSearch'
const route = useRoute(), router = useRouter()
const open = ref(false), query = ref(''), scope = ref<ResourceKind | 'all'>('all'), active = ref(0)
const loading = ref(false), error = ref(''), entries = ref<SearchEntry[]>([])
const trigger = ref<HTMLButtonElement>(), input = ref<HTMLInputElement>(), popup = ref<HTMLElement>()
const currentKind = computed<ResourceKind | undefined>(() => route.path.startsWith('/kamera') || route.path === '/einrichtung' ? 'Kameras' : route.path.includes('/relays') ? 'Relays' : route.path.includes('/identitaeten') ? 'Identitäten' : undefined)
const results = computed(() => searchResources(entries.value, query.value, scope.value === 'all' ? undefined : scope.value))
watch([query, scope], () => { active.value = 0 })
watch(() => route.fullPath, close)
async function load() {
  loading.value = true; error.value = ''
  try {
    const [identities, status] = await Promise.all([api.credentialIdentities(), api.status()])
    entries.value = [
      ...status.devices.filter(device => isCameraResource(device,status.bindings)).map(device => ({ id: device.id, kind: 'Kameras' as const, title: status.bindings?.find(binding => binding.device_id === device.id)?.label || device.hostname || device.model || device.last_ip || device.id, detail: [device.last_ip, device.manufacturer].filter(Boolean).join(' · '), href: `/kamera/${encodeURIComponent(device.id)}` })),
      ...(status.relays || []).map(relay => ({ id: relay.id, kind: 'Relays' as const, title: relay.name || relay.id, detail: relay.ssh_target || relay.host, href: `/system/relays/${encodeURIComponent(relay.id)}` })),
      ...identities.map(identity => ({ id: identity.id, kind: 'Identitäten' as const, title: identity.name, detail: identity.username, href: `/system/identitaeten/${encodeURIComponent(identity.id)}` }))
    ]
  } catch { error.value = 'Suche konnte nicht geladen werden.' }
  finally { loading.value = false }
}
async function toggle() {
  if (open.value) { close(); return }
  open.value = true; scope.value = 'all'; active.value = 0
  await nextTick(); input.value?.focus(); void load()
}
function close() { if (!open.value) return; open.value = false; trigger.value?.focus() }
function move(step: number) { if (results.value.length) active.value = (active.value + step + results.value.length) % results.value.length; void nextTick(() => document.getElementById(`search-${active.value}`)?.scrollIntoView({ block: 'nearest' })) }
function choose() { const entry = results.value[active.value]; if (entry) void navigate(entry.href) }
async function navigate(href: string) { await router.push(href); close() }
function shortcut(event: KeyboardEvent) { if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k' && !document.querySelector('dialog[open]')) { event.preventDefault(); if (open.value) input.value?.focus(); else void toggle() } }
function outside(event: PointerEvent) { if (open.value && !popup.value?.contains(event.target as Node) && !trigger.value?.contains(event.target as Node)) close() }
function trap(event: KeyboardEvent) {
  if (event.key !== 'Tab') return
  const elements = popup.value?.querySelectorAll<HTMLElement>('input,button,a')
  const first = elements?.[0], last = elements?.[elements.length - 1]
  if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
}
onMounted(() => { window.addEventListener('keydown', shortcut); window.addEventListener('pointerdown', outside) })
onBeforeUnmount(() => { window.removeEventListener('keydown', shortcut); window.removeEventListener('pointerdown', outside) })
</script>
