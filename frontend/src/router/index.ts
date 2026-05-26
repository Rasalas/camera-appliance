import { createRouter, createWebHistory } from 'vue-router'
import DashboardPage from '../pages/DashboardPage.vue'
import SetupPage from '../pages/SetupPage.vue'
import CamerasPage from '../pages/CamerasPage.vue'
import DeviceDetailsPage from '../pages/DeviceDetailsPage.vue'
import AssignPage from '../pages/AssignPage.vue'
import DisplayPage from '../pages/DisplayPage.vue'
import SettingsPage from '../pages/SettingsPage.vue'
import EventsPage from '../pages/EventsPage.vue'
import BackupPage from '../pages/BackupPage.vue'

export default createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'dashboard', component: DashboardPage },
    { path: '/setup', name: 'setup', component: SetupPage },
    { path: '/cameras', name: 'cameras', component: CamerasPage },
    { path: '/cameras/:id', name: 'camera-details', component: DeviceDetailsPage },
    { path: '/display', name: 'display', component: DisplayPage },
    { path: '/discovery', redirect: '/cameras' },
    { path: '/devices/:id', redirect: (to) => `/cameras/${to.params.id}` },
    { path: '/assign/:deviceId?', name: 'assign', component: AssignPage },
    { path: '/bindings', redirect: '/display' },
    { path: '/settings', name: 'settings', component: SettingsPage },
    { path: '/events', name: 'events', component: EventsPage },
    { path: '/backup', name: 'backup', component: BackupPage }
  ]
})
