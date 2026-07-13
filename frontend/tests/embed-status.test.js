import assert from 'node:assert/strict'
import test from 'node:test'

import { streamErrorPresentation } from '../public/embed-status.js'

test('turns low-level network errors into a concise camera message', () => {
  assert.deepEqual(
    streamErrorPresentation('mse: streams: dial tcp 192.168.178.35:554: connect: no route to host'),
    {
      title: 'Kamera nicht erreichbar',
      detail: 'Die Netzwerkverbindung fehlt. Ein neuer Verbindungsversuch läuft automatisch.'
    }
  )
})

test('distinguishes authentication failures from generic stream errors', () => {
  assert.equal(streamErrorPresentation('401 Unauthorized').title, 'Anmeldung fehlgeschlagen')
  assert.equal(streamErrorPresentation('decoder failed').title, 'Stream nicht verfügbar')
})
