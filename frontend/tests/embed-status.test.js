import assert from 'node:assert/strict'
import test from 'node:test'

import { playerRetryDelay, streamErrorPresentation } from '../public/embed-status.js'

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

test('retries loading the player with capped exponential backoff', () => {
  assert.deepEqual([0, 1, 2, 3, 4, 5, 6, 50].map((n) => playerRetryDelay(n)), [1000, 2000, 4000, 8000, 16000, 30000, 30000, 30000])
  assert.equal(playerRetryDelay(-1), 1000)
  assert.equal(playerRetryDelay(Number.NaN), 1000)
})
