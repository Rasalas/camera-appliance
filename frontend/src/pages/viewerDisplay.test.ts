import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

test('rotated remote frames do not depend on container query units', () => {
  const viewer = readFileSync(new URL('./ViewerPage.vue', import.meta.url), 'utf8')
  assert.equal(/100cq[wh]/.test(viewer), false, 'rotated dimensions must work without container units')
})

test('tablet portrait mode preserves the configured mosaic', () => {
  const viewer = readFileSync(new URL('./ViewerPage.vue', import.meta.url), 'utf8')
  assert.equal(/width: 100% !important/.test(viewer), false, 'portrait mode must retain mosaic dimensions')
})

import { frameDisplayStyle } from './viewerDisplay.ts'

for (const rotation of [90, 270] as const) {
  test(`quarter turn ${rotation} swaps the measured axes on tablets and after resize`, () => {
    const display = { rotation, mirror: false, flip: false, fit_mode: 'contain' as const, crop: { x: 0, y: 0, width: 100, height: 100 } }
    for (const size of [{ width: 220, height: 710 }, { width: 710, height: 220 }]) {
      const style = frameDisplayStyle(display, size)
      assert.equal(style.width, `${size.height}px`)
      assert.equal(style.height, `${size.width}px`)
      assert.ok(style.transform.includes(`rotate(${rotation}deg)`))
    }
    assert.equal(frameDisplayStyle(display).width, '100%')
    const cropped = frameDisplayStyle({ ...display, crop: { x: 10, y: 20, width: 50, height: 50 } }, { width: 220, height: 710 })
    assert.equal(cropped.width, '1420px')
    assert.equal(cropped.height, '440px')
    assert.ok(cropped.transform.includes('translate(15%, 5%)'))
  })
}
