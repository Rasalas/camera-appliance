import type { AutoBackupStatus, BackupKind, BackupList } from '../types/index.ts'

const kindLabels: Record<BackupKind, string> = {
  manual: 'Von Hand',
  automatic: 'Automatisch',
  'before-update': 'Vor einem Update'
}

export function backupKindLabel(kind: string): string {
  return kindLabels[kind as BackupKind] ?? 'Unbekannt'
}

export function backupSizeLabel(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 1024) return `${Math.max(0, Math.round(bytes || 0))} B`
  const units = ['KB', 'MB', 'GB']
  let value = bytes / 1024, unit = 0
  while (value >= 1024 && unit < units.length - 1) { value /= 1024; unit++ }
  return `${value.toLocaleString('de-DE', { maximumFractionDigits: value < 10 ? 1 : 0 })} ${units[unit]}`
}

export function backupTimeLabel(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'Unbekannte Zeit'
  return date.toLocaleString('de-DE', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}

export function autoBackupSummary(auto: AutoBackupStatus | undefined): string {
  if (!auto) return ''
  if (!auto.enabled) return 'Automatische Sicherung ist aus.'
  const last = auto.last_at ? `Zuletzt: ${backupTimeLabel(auto.last_at)}.` : 'Die erste automatische Sicherung folgt in Kürze.'
  return `Einmal am Tag. Die letzten ${auto.keep} automatischen Sicherungen bleiben erhalten. ${last}`
}

// The daily job never prunes other kinds; each kind is capped when a new one of it is made.
export function otherRetentionText(retention: BackupList['retention'] | undefined): string {
  const manual = retention?.manual, update = retention?.['before-update']
  if (!manual || !update) return ''
  if (manual === update) return `Von Sicherungen von Hand und vor Updates bleiben jeweils die letzten ${manual} erhalten.`
  return `Von Sicherungen von Hand bleiben die letzten ${manual} erhalten, von Sicherungen vor Updates die letzten ${update}.`
}

export function backupDownloadPath(name: string): string {
  return `/api/backups/${encodeURIComponent(name)}`
}
