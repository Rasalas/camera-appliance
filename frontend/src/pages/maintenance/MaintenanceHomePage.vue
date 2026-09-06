<template>
  <nav class="maintenance-destinations" aria-label="Wartungsbereiche">
    <RouterLink v-for="item in destinations" :key="item.to" :to="item.to" class="maintenance-destination">
      <AppIcon :name="item.icon" />
      <span><strong>{{ item.label }}</strong><small>{{ descriptions[item.to] }}</small></span>
      <AppIcon name="chevron" />
    </RouterLink>
  </nav>
</template>
<script setup lang="ts">
import AppIcon from '../../components/AppIcon.vue'
import { maintenancePages, type NavItem } from '../../navigation'
const destinations: NavItem[] = [...maintenancePages, { to: '/system/ueber#updates', label: 'Updates', icon: 'update' }]
const descriptions: Record<string, string> = {
  '/system/wartung/watchdog': 'Kameras und Verbindungen überwachen, automatische Wiederherstellung einstellen.',
  '/system/wartung/sicherung': 'Einstellungen sichern oder aus einem Backup wiederherstellen.',
  '/system/ueber#updates': 'Installierte Version ansehen und verfügbare Updates unter Über Watchdeck prüfen.',
  '/system/wartung/support': 'Protokoll ansehen, Diagnosedaten herunterladen und Hilfe anfragen.',
}
</script>
<style scoped>
.maintenance-destinations { max-width:900px; }
.maintenance-destination { display:flex;align-items:center;gap:20px;padding:24px 12px;border-bottom:1px solid var(--hairline); }
.maintenance-destination > span { flex:1;min-width:0;display:grid;gap:8px; }
.maintenance-destination strong { font-size:16px;font-weight:500; }
.maintenance-destination small { color:var(--ink-mute);font-size:14px;line-height:1.6; }
.maintenance-destination:hover { background:var(--surface);color:var(--live); }
@media(max-width:820px) { .maintenance-destination { gap:14px;padding:22px 0; } }
</style>
