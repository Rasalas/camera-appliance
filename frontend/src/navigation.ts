import type { IconName } from './icons'
export interface NavItem { to: string; label: string; icon: IconName; shortcut?: string }
export const cameraPages: NavItem[] = [
  { to: '/einrichtung', label: 'Kameras', icon: 'camera', shortcut: '2' },
  { to: '/kameras/bild-upload', label: 'Bild-Upload', icon: 'upload' }
]
export const systemPages: NavItem[] = [
  { to: '/system/allgemein', label: 'Allgemein', icon: 'settings' },
  { to: '/system/zugriff', label: 'Zugriff', icon: 'shield' },
  { to: '/system/relays', label: 'Relays', icon: 'relay' },
  { to: '/system/identitaeten', label: 'Identitäten', icon: 'identity' }
]
export const maintenancePages: NavItem[] = [
  { to: '/system/wartung/watchdog', label: 'Watchdog', icon: 'activity' },
  { to: '/system/wartung/sicherung', label: 'Sicherung', icon: 'backup' },
  { to: '/system/wartung/updates', label: 'Updates', icon: 'update' },
  { to: '/system/wartung/support', label: 'Support', icon: 'support' },
  { to: '/system/wartung/ereignisse', label: 'Ereignisprotokoll', icon: 'log' }
]
export const aboutPage: NavItem = { to: '/system/ueber', label: 'Über Watchdeck', icon: 'info' }
export function isNavItemActive(destination: string, path: string): boolean {
  if (destination === '/einrichtung') return path === destination || path.startsWith('/kamera/')
  return path === destination || (destination !== '/' && path.startsWith(destination + '/'))
}
