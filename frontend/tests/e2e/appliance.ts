import type { Page } from '@playwright/test'
import { readFileSync } from 'node:fs'

export async function appliance(page: Page) {
  const devices = ['hof', 'garage'].map((id, i) => ({ id, name: id === 'hof' ? 'Hof' : 'Garage', manufacturer: 'Test', model: 'Kamera', last_ip: `192.0.2.${i + 10}` }))
  const slots = devices.map((device, i) => ({ id: `cam${i + 1}`, label: device.name, role: 'camera', default_stream: 'stream2', sort_order: i }))
  const bindings = devices.map((device, i) => ({ device_id: device.id, slot_id: slots[i].id, enabled: true, username: 'camera-user', stream_name: 'stream2', device }))
  const settings: Record<string, string> = { 'viewer.performance.mode': 'quality', 'viewer.layout.mosaic': JSON.stringify({ type: 'split', dir: 'row', ratio: .5, a: { type: 'leaf', slot: 'cam1' }, b: { type: 'leaf', slot: 'cam2' } }) }
  const writes: Record<string, string>[] = []
  const configurationWrites: unknown[] = []
  const imageWrites: { path: string; body: any }[] = []
  const imageSettings: Record<string, any> = {}
  const auth = { enabled: true, authenticated: true, role: 'admin', admin_password_set: true, viewer_password_set: false, viewer_public: false, local_admin_bypass: false, session_hours: 12 }
  const frame = readFileSync(new URL('../fixtures/courtyard.svg', import.meta.url)).toString('base64')
  let failImageSaves = false
  let failApply = false
  await page.route('**/embed.html?*', route => route.fulfill({ contentType: 'text/html', body: '<!doctype html><title>Fixture camera</title>' }))
  await page.route(url => url.pathname.startsWith('/api/'), async route => {
    const req = route.request(), path = new URL(req.url()).pathname, method = req.method()
    let data: any = {}
    if (path === '/api/auth/status') data = auth
    else if (path === '/api/health') data = { status: 'ok', version: 'fixture', commit: 'fixture' }
    else if (path === '/api/settings') {
      if (method === 'PUT') { const patch = req.postDataJSON(); writes.push(patch); Object.assign(settings, patch) }
      data = settings
    } else if (path === '/api/devices') data = devices
    else if (devices.some(d => path === `/api/devices/${d.id}`)) data = devices.find(d => path.endsWith('/' + d.id))
    else if (path.endsWith('/configuration')) {
      const body = req.postDataJSON()
      configurationWrites.push(body)
      if (body.visible === false) {
        const index = bindings.findIndex(binding => path.includes(`/devices/${binding.device_id}/`))
        if (index >= 0) bindings.splice(index, 1)
      }
      data = { persisted: true, password_persisted: false, applied: !failApply, restarted: !failApply, ...(failApply ? { error: 'Videodienst konnte nicht gestartet werden.' } : {}) }
    } else if (path.endsWith('/credentials')) data = { username: 'camera-user', stream: 'stream2', password_set: true }
    else if (path === '/api/bindings') data = bindings
    else if (path === '/api/slots') data = slots
    else if (path === '/api/relays/status' || path === '/api/credential-identities' || path === '/api/events') data = []
    else if (path === '/api/status') data = { devices, slots, bindings, relays: [], system: { go2rtc: { online: true }, camera_appliance: { online: true } }, watchdog: { enabled: false }, version: { version: 'fixture' }, recent_events: [], scan_runs: [] }
    else if (path === '/api/system/update/status') data = { phase: 'idle' }
    else if (path === '/api/system/update/check') data = { phase: 'up_to_date', current_version: 'fixture' }
    else if (path === '/api/viewer') data = {
      profiles: {}, profile_fits: {}, layout: { mosaic: settings['viewer.layout.mosaic'] }, performance: { mode: 'quality' },
      slots: slots.map((slot, i) => ({ slot, alias: slot.id, label: slot.label, state: 'online', binding: bindings[i], device: devices[i], playback: { page_url: `/embed.html?src=${slot.id}` }, display: { rotation: Number(settings[`camera.display.${devices[i].id}.rotation`] || 0), mirror: false, flip: false, fit_mode: 'contain', crop: { x: 0, y: 0, width: 100, height: 100 } } }))
    }
    else if (path.endsWith('/reference-image')) return route.fulfill({ status: 404 })
    else if (path.endsWith('/frame')) data = { content_type: 'image/svg+xml', image_base64: frame, captured_at: '2026-10-29T12:00:00+01:00' }
    else if (path === '/api/snapshot-upload') data = { protocol: 'sftp', host: 'images.example.test', port: 22, username: 'uploads', directory: '.', password_set: true }
    else if (path.endsWith('/upload-crop')) data = { enabled: false, x: 0, y: 0, width: 100, height: 100 }
    else if (path.endsWith('/upload-image-settings')) {
      if (method === 'PUT') {
        imageWrites.push({ path, body: req.postDataJSON() })
        if (failImageSaves) return route.fulfill({ status: 500, json: { error: 'Privatbereiche konnten nicht gespeichert werden.' } })
        imageSettings[path] = req.postDataJSON()
      }
      data = imageSettings[path] || { masks: [], timestamp: false }
    } else if (path.endsWith('/upload-naming')) data = { mode: 'unique', filename: '', directory: '' }
    else if (path.endsWith('/upload-schedule')) data = { interval_minutes: 0, quiet_hours: { enabled: false, start: '22:00', end: '07:00' } }
    else throw new Error(`Unexpected fixture request: ${method} ${path}`)
    await route.fulfill({ json: data })
  })
  return { settings, writes, configurationWrites, imageSettings, imageWrites, failImageSaves: (value: boolean) => { failImageSaves = value }, failApply: (value: boolean) => { failApply = value } }
}
