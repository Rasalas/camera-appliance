<template>
  <AdminDialog :open="discardOpen" title="Änderungen verwerfen?" compact @close="finishDiscard(false)">
    <p>Deine ungespeicherten Änderungen gehen verloren.</p>
    <div class="form-actions"><button class="btn" @click="finishDiscard(false)">Weiter bearbeiten</button><button class="btn danger" @click="finishDiscard(true)">Verwerfen</button></div>
  </AdminDialog>
</template>
<script setup lang="ts">
import AdminDialog from './AdminDialog.vue'
import { discardOpen, finishDiscard } from '../composables/discardChanges'
</script>
