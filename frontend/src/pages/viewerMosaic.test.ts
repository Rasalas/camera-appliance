import assert from 'node:assert/strict'
import test from 'node:test'

import { defaultMosaicTree, dockCamera, layoutGeometry, parseMosaic, reconcileTree, ROOT_TARGET, setRatioAtPath, treeSlots } from './viewerMosaic.ts'

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

test('docking preserves every camera and does not mutate the saved tree', () => {
  const tree = defaultMosaicTree(['A', 'B', 'C'])
  const saved = JSON.stringify(tree)
  const docked = dockCamera(tree, 'C', ROOT_TARGET, 'left')
  assert.deepEqual(treeSlots(docked).sort(), ['A', 'B', 'C'])
  const pane = layoutGeometry(docked).leaves.find((item) => item.alias === 'C')
  assert.deepEqual(pane?.rect, { x: 0, y: 0, w: 50, h: 100 })
  assert.equal(JSON.stringify(tree), saved)
  assert.deepEqual(parseMosaic(JSON.stringify(docked)), docked)
})

test('reconciliation removes missing cameras and resizing keeps panes visible', () => {
  const tree = reconcileTree(defaultMosaicTree(['A', 'B', 'C']), ['A', 'D'])
  assert.deepEqual(treeSlots(tree).sort(), ['A', 'D'])
  const resized = setRatioAtPath(tree, '', 10)
  const panes = layoutGeometry(resized).leaves
  assert.ok(panes.every((pane) => pane.rect.w > 0 && pane.rect.h > 0))
  assert.equal(panes.reduce((sum, pane) => sum + pane.rect.w * pane.rect.h, 0), 10000)
})

test('four plus portrait preserves every camera and gives the fifth a full-height pane', async () => {
  const { portraitGridTree } = await import('./viewerMosaic.ts')
  const aliases = ['cam1', 'cam2', 'cam3', 'cam4', 'cam5']
  const tree = portraitGridTree(aliases, 'cam5')
  const leaves = layoutGeometry(tree).leaves
  assert.deepEqual(treeSlots(tree), aliases)
  const portrait = leaves.find((pane) => pane.alias === 'cam5')!.rect
  assert.equal(portrait.h, 100)
  assert.equal(portrait.y, 0)
  assert.ok(portrait.x > 70 && portrait.x < 80)
  assert.equal(leaves[0].rect.y, leaves[1].rect.y)
  assert.equal(leaves[2].rect.y, leaves[3].rect.y)
  assert.equal(leaves[2].rect.y, 50)
  assert.ok(Math.abs(leaves.reduce((sum, pane) => sum + pane.rect.w * pane.rect.h, 0) - 10000) < 0.001)
  assert.deepEqual(portraitGridTree(['cam1'], 'cam1'), defaultMosaicTree(['cam1']))
})
