<template>
  <Card>
    <div class="card-row">
      <div>
        <h3>{{ binding.slot_id }} - {{ binding.label || binding.slot?.label || 'Kamera' }}</h3>
        <p>{{ binding.device?.last_ip ? `IP: ${binding.device.last_ip}` : 'Keine aktuelle IP' }}</p>
        <p>Stream: {{ binding.stream_name }}</p>
      </div>
      <StatusBadge :online="Boolean(binding.device?.last_ip)" />
    </div>
  </Card>
</template>

<script setup lang="ts">
import type { Binding } from '../types'
import Card from './Card.vue'
import StatusBadge from './StatusBadge.vue'

defineProps<{ binding: Binding }>()
</script>
