import { createRouter, createWebHistory } from 'vue-router'
import OverviewPage from '../pages/OverviewPage.vue'
import SetupPage from '../pages/SetupPage.vue'
import SystemPage from '../pages/SystemPage.vue'
import DeviceDetailsPage from '../pages/DeviceDetailsPage.vue'

export default createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'overview', component: OverviewPage },
    { path: '/einrichtung', name: 'setup', component: SetupPage },
    { path: '/system', name: 'system', component: SystemPage },
    { path: '/kamera/:id', name: 'device', component: DeviceDetailsPage },

    // legacy redirects from the previous IA
    { path: '/setup', redirect: '/einrichtung' },
    { path: '/cameras', redirect: '/einrichtung' },
    { path: '/cameras/:id', redirect: (to) => `/kamera/${to.params.id}` },
    { path: '/display', redirect: '/einrichtung' },
    { path: '/discovery', redirect: '/einrichtung' },
    { path: '/assign/:deviceId?', redirect: '/einrichtung' },
    { path: '/bindings', redirect: '/einrichtung' },
    { path: '/devices/:id', redirect: (to) => `/kamera/${to.params.id}` },
    { path: '/settings', redirect: '/system' },
    { path: '/events', redirect: '/system' },
    { path: '/backup', redirect: '/system' }
  ]
})
