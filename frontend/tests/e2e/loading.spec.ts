import { test, expect } from '@playwright/test'
import { appliance } from './appliance'

test('system settings become editable while service status is still pending', async ({ page }) => {
  await appliance(page)
  let release!: () => void
  const pending = new Promise<void>(resolve => { release = resolve })
  let requested = false
  await page.route('**/api/status', async route => { requested = true; await pending; await route.fallback() })
  try {
    await page.goto('/system')
    await expect.poll(() => requested).toBe(true)
    await page.getByRole('button', { name: 'Anzeige bearbeiten', exact: true }).click()
    await expect(page.getByRole('dialog', { name: 'Anzeige bearbeiten' }).getByLabel('Performance', { exact: true })).toBeEnabled()
  } finally { release() }
})

test('device details start independent reads concurrently', async ({ page }) => {
  await appliance(page)
  let release!: () => void
  const pending = new Promise<void>(resolve => { release = resolve })
  const requests = new Set<string>()
  page.on('request', req => { if (new URL(req.url()).pathname.startsWith('/api/')) requests.add(new URL(req.url()).pathname) })
  await page.route('**/api/devices/hof', async route => { await pending; await route.fallback() })
  try {
    await page.goto('/kamera/hof/bearbeiten')
    await expect.poll(() => ['/api/devices/hof/credentials', '/api/credential-identities', '/api/bindings', '/api/settings', '/api/relays/status'].every(path => requests.has(path))).toBe(true)
    release()
    await expect(page.getByLabel('Benutzername', { exact: true })).toHaveValue('camera-user')
  } finally { release() }
})

test('40MP previews stay bounded and remain hidden until privacy settings are painted', async ({ page }) => {
  await appliance(page)
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="8000" height="5000"><rect width="8000" height="5000" fill="#00ffff"/><circle cx="5000" cy="3000" r="1500" fill="#ff0000"/></svg>'
  await page.route('**/api/devices/hof/frame', route => route.fulfill({ json: { content_type: 'image/svg+xml', image_base64: Buffer.from(svg).toString('base64'), captured_at: '2026-10-29T12:00:00+01:00' } }))
  let release!: () => void
  const pending = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/devices/hof/upload-image-settings', async route => {
    await pending
    await route.fulfill({ json: { timestamp: true, masks: [
      { id: 'private', mode: 'black', x: 10, y: 10, width: 20, height: 20 },
      { id: 'pixelated', mode: 'pixelate', x: 50, y: 50, width: 20, height: 20 }
    ] } })
  })
  await page.addInitScript(() => {
    const observations: boolean[] = []
    ;(window as any).privacyFrames = observations
    function observe() {
      const canvas = document.querySelector<HTMLCanvasElement>('.image-editor canvas')
      if (canvas && canvas.getBoundingClientRect().height > 0) {
        const pixel = canvas.getContext('2d')!.getImageData(Math.floor(canvas.width * .15), Math.floor(canvas.height * .15), 1, 1).data
        observations.push(pixel[0] === 0 && pixel[1] === 0 && pixel[2] === 0 && pixel[3] === 255)
      }
      requestAnimationFrame(observe)
    }
    requestAnimationFrame(observe)
  })
  try {
    await page.goto('/kamera/hof/bild-upload')
    const canvas = page.getByRole('img', { name: 'Kameravorschau Hof mit Privatbereichen', includeHidden: true })
    await expect(canvas).toBeAttached()
    await expect(canvas).not.toBeVisible()
    release()
    await expect(canvas).toBeVisible()
    expect(await canvas.evaluate((el: HTMLCanvasElement) => ({ width: el.width, height: el.height }))).toEqual({ width: 960, height: 600 })
    const pixels = await canvas.evaluate((el: HTMLCanvasElement) => {
      const ctx = el.getContext('2d')!
      const stamp = ctx.getImageData(768, 573, 187, 22).data
      let white = 0
      for (let i = 0; i < stamp.length; i += 4) if (stamp[i] > 200 && stamp[i + 1] > 200 && stamp[i + 2] > 200) white++
      return { mask: Array.from(ctx.getImageData(144, 90, 1, 1).data), white }
    })
    expect(pixels.mask).toEqual([0, 0, 0, 255])
    expect(pixels.white).toBeGreaterThan(20)
    await expect.poll(() => page.evaluate(() => (window as any).privacyFrames.length)).toBeGreaterThan(0)
    expect(await page.evaluate(() => (window as any).privacyFrames.every(Boolean))).toBe(true)
  } finally { release() }
})
