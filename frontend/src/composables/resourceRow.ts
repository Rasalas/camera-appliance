export function rowDestination(event: MouseEvent, href: string): string | undefined {
  if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || window.getSelection()?.toString()) return
  if ((event.target as HTMLElement).closest('a,button,input,select,textarea,[role=button]')) return
  return href
}
