import assert from 'node:assert/strict'
import test from 'node:test'

import { defaultMosaicTree } from './viewerMosaic.ts'

test('creates the expected automatic layouts for one through four cameras', () => {
  assert.deepEqual(defaultMosaicTree(['A']), { type: 'leaf', slot: 'A' })
  assert.deepEqual(defaultMosaicTree(['A', 'B']), {
    type: 'split', dir: 'row', ratio: 0.5,
    a: { type: 'leaf', slot: 'A' },
    b: { type: 'leaf', slot: 'B' }
  })
  assert.deepEqual(defaultMosaicTree(['A', 'B', 'C']), {
    type: 'split', dir: 'row', ratio: 0.5,
    a: {
      type: 'split', dir: 'col', ratio: 0.5,
      a: { type: 'leaf', slot: 'A' },
      b: { type: 'leaf', slot: 'C' }
    },
    b: { type: 'leaf', slot: 'B' }
  })
  assert.deepEqual(defaultMosaicTree(['A', 'B', 'C', 'D']), {
    type: 'split', dir: 'col', ratio: 0.5,
    a: {
      type: 'split', dir: 'row', ratio: 0.5,
      a: { type: 'leaf', slot: 'A' },
      b: { type: 'leaf', slot: 'B' }
    },
    b: {
      type: 'split', dir: 'row', ratio: 0.5,
      a: { type: 'leaf', slot: 'C' },
      b: { type: 'leaf', slot: 'D' }
    }
  })
})
