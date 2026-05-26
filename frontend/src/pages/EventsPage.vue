<template>
  <PageHeader title="Logs und Ereignisse" subtitle="Letzte lokale Vorgänge" />
  <Card>
    <EmptyState v-if="events.length === 0" text="Noch keine Ereignisse vorhanden." />
    <table v-else>
      <thead><tr><th>Zeit</th><th>Level</th><th>Typ</th><th>Meldung</th></tr></thead>
      <tbody>
        <tr v-for="event in events" :key="event.id">
          <td>{{ new Date(event.created_at).toLocaleString('de-DE') }}</td>
          <td>{{ event.level }}</td>
          <td>{{ event.type }}</td>
          <td>{{ event.message }}</td>
        </tr>
      </tbody>
    </table>
  </Card>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'
import type { EventItem } from '../types'
import PageHeader from '../components/PageHeader.vue'
import Card from '../components/Card.vue'
import EmptyState from '../components/EmptyState.vue'

const events = ref<EventItem[]>([])
onMounted(async () => {
  events.value = await api.events()
})
</script>
