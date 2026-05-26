<template>
  <Card>
    <div class="card-row">
      <div>
        <h3>{{ title }}</h3>
        <p>IP: {{ device.last_ip || 'unbekannt' }}</p>
        <p>MAC: {{ device.mac_address || 'unbekannt' }}</p>
        <p>Hostname: {{ device.hostname || 'unbekannt' }}</p>
      </div>
      <StatusBadge :online="Boolean(device.last_ip)" />
    </div>
    <div class="button-row">
      <RouterLink class="button-link" :to="`/assign/${device.id}`">Zuordnen</RouterLink>
      <RouterLink class="button-link secondary" to="/bindings">Details</RouterLink>
    </div>
  </Card>
</template>

<script setup lang="ts">
import type { Device } from '../types'
import Card from './Card.vue'
import StatusBadge from './StatusBadge.vue'

const props = defineProps<{ device: Device }>()
const title = `${props.device.manufacturer || 'Unbekanntes'} ${props.device.model || 'RTSP-Gerät'}`.trim()
</script>
