<template>
  <RouterLink :to="item.to" custom v-slot="{ href, navigate }">
    <a :href="href" :class="{ 'nav-active': active, 'nav-parent-active': parentActive }" :aria-current="active ? 'page' : undefined" @click="navigate"><AppIcon :name="item.icon" /><span>{{ item.label }}</span><kbd v-if="item.shortcut" class="nav-key" aria-hidden="true">{{ item.shortcut }}</kbd></a>
  </RouterLink>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { isNavItemActive, isNavItemAncestor, type NavItem } from '../navigation'
import AppIcon from './AppIcon.vue'
const props = defineProps<{ item: NavItem }>()
const route = useRoute()
const active = computed(() => isNavItemActive(props.item.to, route.path))
const parentActive = computed(() => isNavItemAncestor(props.item.to, route.path))
</script>
