<template>
  <header class="topline"><h1 class="headline">Home</h1></header>
  <p class="mono-mute">Kameras verwalten, die Live-Ansicht öffnen oder die Station warten.</p>
  <section v-for="group in groups" :key="group.title" class="home-group"><h2>{{ group.title }}</h2><RouterLink v-for="item in group.items" :key="item.to" :to="item.to">{{ item.label }}<span aria-hidden="true">→</span></RouterLink></section>
</template>
<script setup lang="ts">
import { systemPages, maintenancePages } from '../navigation'
const groups = [
  { title: 'Kameras', items: [{to:'/',label:'Live-Ansicht'},{to:'/einrichtung',label:'Kameras'},{to:'/kameras/bild-upload',label:'Bild-Upload'}] },
  { title: 'System', items: systemPages },
  { title: 'Wartung', items: maintenancePages }
]
</script>
