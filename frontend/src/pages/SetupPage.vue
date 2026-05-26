<template>
  <PageHeader title="Einrichtung" subtitle="Kameras suchen und festen Anzeigeplätzen zuordnen" />
  <Card>
    <p>Dieser Assistent sucht Kameras im lokalen Netzwerk und ordnet sie festen Anzeigeplätzen zu.</p>
    <div class="summary-list">
      <div><strong>{{ slots.length }}</strong><span>Plätze konfiguriert</span></div>
      <div><strong>{{ bindings.length }}</strong><span>Kameras zugeordnet</span></div>
      <div><strong>{{ missing }}</strong><span>Kameras fehlen</span></div>
    </div>
    <RouterLink class="action-button primary" to="/discovery">Suche starten</RouterLink>
  </Card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api/client'
import type { Binding, Slot } from '../types'
import PageHeader from '../components/PageHeader.vue'
import Card from '../components/Card.vue'

const slots = ref<Slot[]>([])
const bindings = ref<Binding[]>([])
const missing = computed(() => Math.max(0, slots.value.length - bindings.value.length))

onMounted(async () => {
  const status = await api.status()
  slots.value = status.slots
  bindings.value = status.bindings
})
</script>
