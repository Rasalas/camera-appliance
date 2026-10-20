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

function fakeApi(restartFails = false) {
  const calls: string[] = []
  return {
    calls,
    api: {
      saveBinding: async (binding: { slot_id: string; label?: string }) => { calls.push(`save ${binding.slot_id} ${binding.label}`) },
      removeBinding: async (slot: string) => { calls.push(`remove ${slot}`) },
      renderGo2RTC: async () => { calls.push('render') },
      restartGo2RTC: async () => { calls.push('restart'); if (restartFails) throw new Error('docker down') }
    }
  }
}
const slots = [{ id: 'cam1', default_stream: 'stream2' }, { id: 'cam2', default_stream: 'stream2' }]

test('showing a camera binds a free slot under its display name and restarts the video service', async () => {
  const { api, calls } = fakeApi()
  const result = await changeCameraVisibility(api, { deviceId: 'd2', label: 'Einfahrt', bindings: [{ slot_id: 'cam1', device_id: 'd1' }], slots })
  assert.deepEqual(calls, ['save cam2 Einfahrt', 'render', 'restart'])
  assert.equal(result.restartError, '')
})

test('hiding a camera reports a failed restart instead of ignoring it', async () => {
  const { api, calls } = fakeApi(true)
  const result = await changeCameraVisibility(api, { deviceId: 'd1', label: 'Hof', bindings: [{ slot_id: 'cam1', device_id: 'd1' }], slots })
  assert.deepEqual(calls, ['remove cam1', 'render', 'restart'])
  assert.match(result.restartError, /Videodienst/)
})

test('showing a camera without a free slot fails before changing anything', async () => {
  const { api, calls } = fakeApi()
  await assert.rejects(changeCameraVisibility(api, { deviceId: 'd3', label: 'Garten', bindings: [{ slot_id: 'cam1', device_id: 'd1' }, { slot_id: 'cam2', device_id: 'd2' }], slots }), /Maximal 2 Kameras/)
  assert.deepEqual(calls, [])
})
