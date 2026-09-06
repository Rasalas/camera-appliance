import { createRouter, createWebHistory } from 'vue-router'
import AboutPage from '../pages/system/AboutPage.vue'
import AdminHomePage from '../pages/AdminHomePage.vue'
import IdentityDetailPage from '../pages/system/IdentityDetailPage.vue'
import ViewerPage from '../pages/ViewerPage.vue'
import SetupPage from '../pages/SetupPage.vue'
import SystemLayout from '../layouts/SystemLayout.vue'
import SystemGeneralPage from '../pages/system/GeneralPage.vue'
import CameraUploadPage from '../pages/CameraUploadPage.vue'
import SystemAccessPage from '../pages/system/AccessPage.vue'
import SystemNetworkPage from '../pages/system/NetworkPage.vue'
import SystemRelayDetailPage from '../pages/system/RelayDetailPage.vue'
import SystemIdentitiesPage from '../pages/system/IdentitiesPage.vue'
import { legacyMaintenanceDestination } from '../navigation'
import MaintenanceHomePage from '../pages/maintenance/MaintenanceHomePage.vue'
import WatchdogPage from '../pages/maintenance/WatchdogPage.vue'
import BackupPage from '../pages/maintenance/BackupPage.vue'
import SupportPage from '../pages/maintenance/SupportPage.vue'
import DeviceDetailsPage from '../pages/DeviceDetailsPage.vue'
import LoginPage from '../pages/LoginPage.vue'
import { api } from '../api/client'
import type { AuthStatus } from '../types'

const router = createRouter({
  history: createWebHistory(),
  scrollBehavior(to, from, saved) { return saved || (to.hash ? { el: to.hash, top: 24 } : { top: 0 }) },
  routes: [
    { path: '/', name: 'viewer', component: ViewerPage, meta: { requiresViewer: true } },
    { path: '/verwaltung', component: AdminHomePage, meta: { requiresAdmin: true } },
    { path: '/login', name: 'login', component: LoginPage },
    { path: '/kameras/bild-upload/bearbeiten', component: CameraUploadPage, meta: { requiresAdmin: true } },
    { path: '/kameras/bild-upload', name: 'camera-upload', component: CameraUploadPage, meta: { requiresAdmin: true } },
    { path: '/einrichtung', name: 'setup', component: SetupPage, meta: { requiresAdmin: true } },
    {
      path: '/system',
      component: SystemLayout,
      meta: { requiresAdmin: true },
      children: [
        { path: '', name: 'system-general', component: SystemGeneralPage, meta: { title: 'System' } },
        { path: 'ueber', component: AboutPage, meta: { title: 'Über Watchdeck' } },
        { path: 'allgemein', redirect: to => ({ path: '/system', query: to.query, hash: to.hash }) },
        { path: 'bild-upload', name: 'system-upload', redirect: '/kameras/bild-upload' },
        { path: 'zugriff', name: 'system-access', component: SystemAccessPage, meta: { title: 'Zugriff' } },
        { path: 'relays', name: 'system-relays', component: SystemNetworkPage, meta: { title: 'Relays' } },
        { path: 'relays/:id', name: 'system-relay', component: SystemRelayDetailPage, meta: { title: 'Relay' } },
        { path: 'netzwerk', redirect: '/system/relays' },
        { path: 'identitaeten', name: 'system-identities', component: SystemIdentitiesPage, meta: { title: 'Identitäten' } },
        { path: 'identitaeten/:id', component: IdentityDetailPage, meta: { title: 'Identität' } },
        { path: 'wartung', name: 'system-maintenance', component: MaintenanceHomePage, meta: { title: 'Wartung' } },
        { path: 'wartung/watchdog', component: WatchdogPage, meta: { title: 'Watchdog' } },
        { path: 'wartung/sicherung', component: BackupPage, meta: { title: 'Sicherung' } },
        { path: 'wartung/updates', redirect: to => ({ path: '/system/ueber', hash: '#updates', query: to.query }) },
        { path: 'wartung/support', component: SupportPage, meta: { title: 'Support' } },
        { path: 'wartung/ereignisse', redirect: to => ({ path: '/system/wartung/support', hash: '#ereignisse', query: to.query }) }
      ]
    },
    { path: '/kamera/:id/bearbeiten', name: 'device-edit', component: DeviceDetailsPage, meta: { requiresAdmin: true } },
    { path: '/kamera/:id/bild-upload', name: 'device-upload', component: DeviceDetailsPage, meta: { requiresAdmin: true } },
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
    { path: '/events', redirect: '/system/wartung/ereignisse' },
    { path: '/backup', redirect: '/system/wartung/sicherung' }
  ]
})

router.beforeEach(async (to) => {
  if (to.path === '/system/wartung') {
    const destination = legacyMaintenanceDestination(to.hash)
    if (destination) return { path: destination, query: to.query, replace: true }
  }
  if (to.path === '/verwaltung' && window.matchMedia('(min-width: 821px)').matches) return { path: '/einrichtung', replace: true }
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
