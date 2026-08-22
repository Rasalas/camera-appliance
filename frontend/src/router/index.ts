import { createRouter, createWebHistory } from 'vue-router'
import ViewerPage from '../pages/ViewerPage.vue'
import SetupPage from '../pages/SetupPage.vue'
import SystemLayout from '../layouts/SystemLayout.vue'
import SystemGeneralPage from '../pages/system/GeneralPage.vue'
import SystemAccessPage from '../pages/system/AccessPage.vue'
import SystemNetworkPage from '../pages/system/NetworkPage.vue'
import SystemRelayDetailPage from '../pages/system/RelayDetailPage.vue'
import SystemIdentitiesPage from '../pages/system/IdentitiesPage.vue'
import SystemMaintenancePage from '../pages/system/MaintenancePage.vue'
import DeviceDetailsPage from '../pages/DeviceDetailsPage.vue'
import LoginPage from '../pages/LoginPage.vue'
import { api } from '../api/client'
import type { AuthStatus } from '../types'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'viewer', component: ViewerPage, meta: { requiresViewer: true } },
    { path: '/login', name: 'login', component: LoginPage },
    { path: '/einrichtung', name: 'setup', component: SetupPage, meta: { requiresAdmin: true } },
    {
      path: '/system',
      component: SystemLayout,
      meta: { requiresAdmin: true },
      children: [
        { path: '', redirect: '/system/allgemein' },
        { path: 'allgemein', name: 'system-general', component: SystemGeneralPage },
        { path: 'zugriff', name: 'system-access', component: SystemAccessPage },
        { path: 'relays', name: 'system-relays', component: SystemNetworkPage },
        { path: 'relays/:id', name: 'system-relay', component: SystemRelayDetailPage },
        { path: 'netzwerk', redirect: '/system/relays' },
        { path: 'identitaeten', name: 'system-identities', component: SystemIdentitiesPage },
        { path: 'wartung', name: 'system-maintenance', component: SystemMaintenancePage }
      ]
    },
    { path: '/kamera/:id', name: 'device', component: DeviceDetailsPage, meta: { requiresAdmin: true } },

    // legacy redirects from the previous IA
    { path: '/uebersicht', redirect: '/einrichtung' },
    { path: '/setup', redirect: '/einrichtung' },
    { path: '/overview', redirect: '/einrichtung' },
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
    // Fail closed: an unreachable auth endpoint (e.g. during a self-update
    // restart) must not grant access to admin routes.
    return to.meta.requiresAdmin || to.meta.requiresViewer ? { path: '/login' } : true
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
