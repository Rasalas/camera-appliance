import { createRouter, createWebHistory } from 'vue-router'
import ViewerPage from '../pages/ViewerPage.vue'
import OverviewPage from '../pages/OverviewPage.vue'
import SetupPage from '../pages/SetupPage.vue'
import SystemPage from '../pages/SystemPage.vue'
import DeviceDetailsPage from '../pages/DeviceDetailsPage.vue'
import LoginPage from '../pages/LoginPage.vue'
import { api } from '../api/client'
import type { AuthStatus } from '../types'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'viewer', component: ViewerPage, meta: { requiresViewer: true } },
    { path: '/login', name: 'login', component: LoginPage },
    { path: '/uebersicht', name: 'overview', component: OverviewPage, meta: { requiresAdmin: true } },
    { path: '/einrichtung', name: 'setup', component: SetupPage, meta: { requiresAdmin: true } },
    { path: '/system', name: 'system', component: SystemPage, meta: { requiresAdmin: true } },
    { path: '/kamera/:id', name: 'device', component: DeviceDetailsPage, meta: { requiresAdmin: true } },

    // legacy redirects from the previous IA
    { path: '/setup', redirect: '/einrichtung' },
    { path: '/overview', redirect: '/uebersicht' },
    { path: '/cameras', redirect: '/einrichtung' },
    { path: '/cameras/:id', redirect: (to) => `/kamera/${to.params.id}` },
    { path: '/display', redirect: '/' },
    { path: '/discovery', redirect: '/einrichtung' },
    { path: '/assign/:deviceId?', redirect: '/einrichtung' },
    { path: '/bindings', redirect: '/einrichtung' },
    { path: '/devices/:id', redirect: (to) => `/kamera/${to.params.id}` },
    { path: '/settings', redirect: '/system' },
    { path: '/events', redirect: '/system' },
    { path: '/backup', redirect: '/system' }
  ]
})

router.beforeEach(async (to) => {
  if (to.name === 'login') return true
  let auth: AuthStatus
  try {
    auth = await api.authStatus()
  } catch {
    return true
  }
  if (!auth.enabled) return true
  if (to.meta.requiresAdmin && auth.role !== 'admin') {
    return { path: '/login', query: { next: to.fullPath } }
  }
  if (to.meta.requiresViewer && !auth.viewer_public && auth.role !== 'admin' && auth.role !== 'viewer') {
    return { path: '/login', query: { next: to.fullPath } }
  }
  return true
})

export default router
