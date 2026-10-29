import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: true,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 2,
  use: { baseURL: 'http://127.0.0.1:18174', trace: 'retain-on-failure' },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
    { name: 'tablet', use: { ...devices['Desktop Chrome'], viewport: { width: 768, height: 1024 }, hasTouch: true } }
  ],
  webServer: [
    { command: 'node tests/e2e/upstream.mjs', url: 'http://127.0.0.1:18175/api/health' },
    {
      command: 'npm run dev -- --host 127.0.0.1 --port 18174 --strictPort',
      url: 'http://127.0.0.1:18174',
      env: { CAMERA_APPLIANCE_DEV_API_TARGET: 'http://127.0.0.1:18175' }
    }
  ]
})
