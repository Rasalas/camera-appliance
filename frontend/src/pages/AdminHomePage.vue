<template>
  <header class="topline"><h1 class="headline">Home</h1></header>
  <p class="mono-mute">Kameras verwalten, die Live-Ansicht öffnen oder die Station warten.</p>
  <section v-for="group in groups" :key="group.title" class="home-group"><h2>{{ group.title }}</h2><RouterLink v-for="item in group.items" :key="item.to" :to="item.to"><AppIcon :name="item.icon" /><span class="home-label">{{ item.label }}</span><span aria-hidden="true">→</span></RouterLink></section>
</template>
<script setup lang="ts">
import { onBeforeUnmount, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import { cameraPages, systemPage, systemPages, maintenancePage, maintenancePages, aboutPage } from '../navigation'
const groups = [
  { title: 'Kameras', items: [{to:'/',label:'Live-Ansicht',icon:'live' as const}, ...cameraPages] },
  { title: 'System', items: [systemPage, ...systemPages] },
  { title: 'Wartung', items: [maintenancePage, ...maintenancePages] },
  { title: 'Projekt', items: [aboutPage] }
]
const router = useRouter()
const desktop = window.matchMedia('(min-width: 821px)')
function leaveDesktopHome() { if (desktop.matches) void router.replace('/einrichtung') }
onMounted(() => { leaveDesktopHome(); desktop.addEventListener('change', leaveDesktopHome) })
onBeforeUnmount(() => desktop.removeEventListener('change', leaveDesktopHome))
</script>
