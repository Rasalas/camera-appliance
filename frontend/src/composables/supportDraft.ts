export function supportMailURL(recipient: string, description: string, diagnostics: string, includeDiagnostics: boolean): string {
  const body = [description.trim() || 'Bitte beschreibe hier das Problem.', includeDiagnostics ? diagnostics : '', 'Ein Support-Bundle bei Bedarf als Anhang hinzufügen.'].filter(Boolean).join('\n\n')
  return `mailto:${recipient}?subject=${encodeURIComponent('Watchdeck · Hilfe anfragen')}&body=${encodeURIComponent(body)}`
}
