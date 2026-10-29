import { test, expect } from '@playwright/test'

test('hot reload forwards player modules and same-origin WebSockets', async ({ page }) => {
  await page.goto('/embed.html')
  expect(await page.evaluate(async () => {
    // Variable import keeps Vite from attempting to resolve this runtime URL.
    const path = '/go2rtc/video-stream.js'
    return (await import(/* @vite-ignore */ path)).proxyMarker
  })).toBe('player-module-from-upstream')
  expect(await page.evaluate(() => new Promise<string>((resolve, reject) => {
    const ws = new WebSocket(`ws://${location.host}/go2rtc/api/ws?src=cam1`)
    ws.onmessage = event => { resolve(String(event.data)); ws.close() }
    ws.onerror = () => { ws.close(); reject(new Error('player WebSocket proxy failed')) }
  }))).toBe('player-socket-from-upstream')
})
