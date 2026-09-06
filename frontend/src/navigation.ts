import type { IconName } from './icons'
export interface NavItem { to: string; label: string; icon: IconName; shortcut?: string }
export const cameraPages: NavItem[] = [
  { to: '/einrichtung', label: 'Kameras', icon: 'camera', shortcut: '2' },
  { to: '/kameras/bild-upload', label: 'Bild-Upload', icon: 'upload' }
]
export const systemPage: NavItem = { to: '/system', label: 'System', icon: 'system', shortcut: '3' }
export const maintenancePage: NavItem = { to: '/system/wartung', label: 'Wartung', icon: 'tools' }
export const systemPages: NavItem[] = [
  { to: '/system/zugriff', label: 'Zugriff', icon: 'shield' },
  { to: '/system/relays', label: 'Relays', icon: 'relay' },
  { to: '/system/identitaeten', label: 'Identitäten', icon: 'identity' }
]
export const maintenancePages: NavItem[] = [
  { to: '/system/wartung/watchdog', label: 'Watchdog', icon: 'activity' },
  { to: '/system/wartung/sicherung', label: 'Sicherung', icon: 'backup' },
  { to: '/system/wartung/support', label: 'Support', icon: 'support' }
]
export const aboutPage: NavItem = { to: '/system/ueber', label: 'Über Watchdeck', icon: 'info' }
export function isNavItemActive(destination: string, path: string): boolean {
  if (destination === systemPage.to || destination === maintenancePage.to) return path === destination
  if (destination === '/einrichtung') return path === destination || path.startsWith('/kamera/')
  return path === destination || (destination !== '/' && path.startsWith(destination + '/'))
}

export function isNavItemAncestor(destination: string, path: string): boolean {
  if (isNavItemActive(destination, path)) return false
  if (destination === systemPage.to) return systemPages.some(item => isNavItemActive(item.to, path))
  if (destination === maintenancePage.to) return maintenancePages.some(item => isNavItemActive(item.to, path))
  return destination === '/einrichtung' && isNavItemActive('/kameras/bild-upload', path)
}

export function legacyMaintenanceDestination(hash: string): string | undefined {
  const pages: Record<string, string> = { '#backup': 'sicherung', '#sicherung': 'sicherung', '#updates': 'updates', '#version': 'updates', '#support': 'support', '#events': 'ereignisse', '#ereignisse': 'ereignisse' }
  return pages[hash] ? '/system/wartung/' + pages[hash] : undefined
}
