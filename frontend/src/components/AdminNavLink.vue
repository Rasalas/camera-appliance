<template>
  <RouterLink :to="item.to" :class="{ 'nav-active': active }" :aria-current="active ? 'page' : undefined"><AppIcon :name="item.icon" /><span>{{ item.label }}</span><kbd v-if="item.shortcut" class="nav-key" aria-hidden="true">{{ item.shortcut }}</kbd></RouterLink>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { isNavItemActive, type NavItem } from '../navigation'
import AppIcon from './AppIcon.vue'
const props = defineProps<{ item: NavItem }>()
const route = useRoute()
const active = computed(() => isNavItemActive(props.item.to, route.path))
</script>
