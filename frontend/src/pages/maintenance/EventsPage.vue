<template>
  <section class="panel card">
    <div class="panel-head">
      <h2>Ereignisprotokoll</h2>
      <div class="right">{{ events.length }} Einträge</div>
    </div>
    <div v-if="!events.length" class="empty">Noch keine Ereignisse vorhanden.</div>
    <div v-else class="ticker">
      <div v-for="ev in events" :key="ev.id" class="row">
        <span class="time">{{ formatTime(ev.created_at) }}</span>
        <span class="lvl" :class="levelClass(ev.level)">{{ ev.level }}</span>
        <span><b style="color: var(--ink); font-weight: 500;">{{ ev.type }}</b> · {{ ev.message }}</span>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useSystem } from '../../composables/useSystem'
const { events, formatTime, levelClass } = useSystem()
</script>
