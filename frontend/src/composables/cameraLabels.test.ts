import test from 'node:test'
import assert from 'node:assert/strict'
import { cameraHardwareLabel, cameraName, cameraNameError, cameraNameMaxLength, cameraSearchDetail, cameraStateLabel, changeCameraVisibility, countLabel, fitModeLabel, streamLabel } from './cameraLabels.ts'

test('camera names prefer the custom name and fall back to manufacturer and model', () => {
  assert.equal(cameraName({ name: '  Einfahrt ', manufacturer: 'Tapo', model: 'C200' }), 'Einfahrt')
  assert.equal(cameraName({ name: '', manufacturer: 'Tapo', model: 'C200' }), 'Tapo C200')
  assert.equal(cameraName({ model: 'C200' }), 'C200')
  assert.equal(cameraName({}), 'Unbekannte Kamera')
  assert.equal(cameraHardwareLabel({ name: 'Einfahrt', manufacturer: 'Tapo', model: 'C200' }), 'Tapo C200')
})

test('camera search covers the custom name, hardware label and address', () => {
  assert.equal(cameraSearchDetail({ name: 'Einfahrt', manufacturer: 'Tapo', model: 'C200', last_ip: '192.0.2.20' }), 'Tapo C200 · 192.0.2.20')
  assert.equal(cameraSearchDetail({ manufacturer: 'Tapo', model: 'C200' }), '')
})

test('camera name validation matches the backend limit', () => {
  assert.equal(cameraNameError('Einfahrt'), '')
  assert.equal(cameraNameError(' '.repeat(5) + 'x'.repeat(cameraNameMaxLength) + ' '), '')
  assert.match(cameraNameError('x'.repeat(cameraNameMaxLength + 1)), /höchstens 40 Zeichen/)
  assert.match(cameraNameError('Hof\nhinten'), /Zeilenumbrüche/)
})

test('counts use proper German singular and plural', () => {
  assert.equal(countLabel(1, 'Kamera', 'Kameras'), '1 Kamera')
  assert.equal(countLabel(0, 'Vorschau', 'Vorschauen'), '0 Vorschauen')
  assert.equal(countLabel(3, 'Gerät', 'Geräte'), '3 Geräte')
})

test('technical values get plain labels', () => {
  assert.equal(cameraStateLabel('credentials_failed'), 'Zugangsdaten falsch')
  assert.equal(cameraStateLabel('stream_unavailable'), 'Kein Bild')
  assert.equal(fitModeLabel('contain'), 'Einpassen')
  assert.equal(fitModeLabel('cover'), 'Ausfüllen')
  assert.equal(streamLabel('stream2'), 'Standard (empfohlen)')
  assert.equal(streamLabel('stream1'), 'Hohe Auflösung')
  assert.equal(streamLabel('custom'), 'custom')
})

function fakeApi(applyFails = false) {
  const calls: unknown[] = []
  return {
    calls,
    api: {
      configureCamera: async (id: string, body: { visible?: boolean }) => {
        calls.push({ id, ...body })
        return { persisted: true, password_persisted: false, applied: !applyFails, restarted: !applyFails }
      }
    }
  }
}

test('showing a camera lets the server allocate its slot and apply its configuration', async () => {
  const { api, calls } = fakeApi()
  const result = await changeCameraVisibility(api, { deviceId: 'd2', bindings: [{ slot_id: 'cam1', device_id: 'd1' }] })
  assert.deepEqual(calls, [{ id: 'd2', visible: true }])
  assert.equal(result.restartError, '')
})

test('a failed apply is retryable without toggling the desired visibility', async () => {
  const { api, calls } = fakeApi(true)
  const input = { deviceId: 'd1', bindings: [{ slot_id: 'cam1', device_id: 'd1' }] }
  const result = await changeCameraVisibility(api, input)
  assert.match(result.restartError, /gespeichert, aber noch nicht angewendet/)
  await changeCameraVisibility(api, input)
  assert.deepEqual(calls, [{ id: 'd1', visible: false }, { id: 'd1', visible: false }])
})

test('slot conflicts from the server are reported without a separate binding write', async () => {
  const api = { configureCamera: async () => { throw new Error('Kein freier Kameraplatz verfügbar.') } }
  await assert.rejects(changeCameraVisibility(api, { deviceId: 'd3', bindings: [] }), /Kein freier Kameraplatz/)
})
