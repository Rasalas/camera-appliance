export function streamErrorPresentation(rawMessage = '') {
  const message = String(rawMessage).toLowerCase()

  if (/unauthorized|authentication|wrong password|401|403/.test(message)) {
    return {
      title: 'Anmeldung fehlgeschlagen',
      detail: 'Bitte die Zugangsdaten dieser Kamera prüfen.'
    }
  }

  if (/no route to host|network is unreachable|connection refused|timed out|timeout|i\/o timeout/.test(message)) {
    return {
      title: 'Kamera nicht erreichbar',
      detail: 'Die Netzwerkverbindung fehlt. Ein neuer Verbindungsversuch läuft automatisch.'
    }
  }

  return {
    title: 'Stream nicht verfügbar',
    detail: 'Der Kamerastream kann gerade nicht geladen werden. Ein neuer Versuch läuft automatisch.'
  }
}
