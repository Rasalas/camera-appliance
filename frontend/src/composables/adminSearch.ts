export type ResourceKind = 'Kameras' | 'Relays' | 'Identitäten'
export interface SearchEntry { id: string; kind: ResourceKind; title: string; detail: string; href: string }
export function searchResources(entries: SearchEntry[], query: string, scope?: ResourceKind): SearchEntry[] {
  const words = query.trim().toLocaleLowerCase('de').split(/\s+/).filter(Boolean)
  return entries.filter(entry => (!scope || entry.kind === scope) && words.every(word => `${entry.title} ${entry.detail}`.toLocaleLowerCase('de').includes(word)))
    .sort((a,b) => a.kind.localeCompare(b.kind, 'de') || a.title.localeCompare(b.title, 'de'))
}
