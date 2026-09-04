export type MosaicLeaf = { type: 'leaf'; slot: string }
export type MosaicSplit = { type: 'split'; dir: 'row' | 'col'; ratio: number; a: MosaicNode; b: MosaicNode }
export type MosaicNode = MosaicLeaf | MosaicSplit

function leaf(slot: string): MosaicLeaf {
  return { type: 'leaf', slot }
}

function evenSplit(nodes: MosaicNode[], dir: 'row' | 'col'): MosaicNode {
  if (nodes.length === 1) return nodes[0]
  return { type: 'split', dir, ratio: 1 / nodes.length, a: nodes[0], b: evenSplit(nodes.slice(1), dir) }
}

export function defaultMosaicTree(aliases: string[]): MosaicNode {
  if (aliases.length <= 1) return leaf(aliases[0] || 'cam1')
  if (aliases.length === 3) {
    return {
      type: 'split',
      dir: 'row',
      ratio: 0.5,
      a: evenSplit([leaf(aliases[0]), leaf(aliases[2])], 'col'),
      b: leaf(aliases[1])
    }
  }

  const columns = Math.ceil(Math.sqrt(aliases.length))
  const rows = Math.ceil(aliases.length / columns)
  const base = Math.floor(aliases.length / rows)
  const extra = aliases.length % rows
  const rowNodes: MosaicNode[] = []
  let index = 0
  for (let row = 0; row < rows; row += 1) {
    const count = base + (row < extra ? 1 : 0)
    const cells = aliases.slice(index, index + count).map(leaf)
    index += count
    if (cells.length) rowNodes.push(evenSplit(cells, 'row'))
  }
  return evenSplit(rowNodes, 'col')
}

export type Rect = { x: number; y: number; w: number; h: number }
export type Side = 'left' | 'right' | 'top' | 'bottom' | 'center'
export type PaneRect = { alias: string; rect: Rect }
export type GutterRect = { id: string; path: string; dir: 'row' | 'col'; rect: Rect; line: Rect }

export const ROOT_TARGET = "__root__"

function computeRects(node: MosaicNode, rect: Rect, path: string, leaves: PaneRect[], gutters: GutterRect[]) {
  if (node.type === 'leaf') {
    leaves.push({ alias: node.slot, rect })
    return
  }
  const ratio = clamp(node.ratio, 0.05, 0.95)
  if (node.dir === 'row') {
    const aw = rect.w * ratio
    const aRect = { x: rect.x, y: rect.y, w: aw, h: rect.h }
    const bRect = { x: rect.x + aw, y: rect.y, w: rect.w - aw, h: rect.h }
    gutters.push({ id: path, path, dir: 'row', rect, line: { x: rect.x + aw, y: rect.y, w: 0, h: rect.h } })
    computeRects(node.a, aRect, path + 'a', leaves, gutters)
    computeRects(node.b, bRect, path + 'b', leaves, gutters)
  } else {
    const ah = rect.h * ratio
    const aRect = { x: rect.x, y: rect.y, w: rect.w, h: ah }
    const bRect = { x: rect.x, y: rect.y + ah, w: rect.w, h: rect.h - ah }
    gutters.push({ id: path, path, dir: 'col', rect, line: { x: rect.x, y: rect.y + ah, w: rect.w, h: 0 } })
    computeRects(node.a, aRect, path + 'a', leaves, gutters)
    computeRects(node.b, bRect, path + 'b', leaves, gutters)
  }
}

export function layoutGeometry(tree: MosaicNode | undefined) {
  const leaves: PaneRect[] = []
  const gutters: GutterRect[] = []
  if (tree) computeRects(tree, { x: 0, y: 0, w: 100, h: 100 }, "", leaves, gutters)
  return { leaves, gutters }
}

export function treeSlots(node: MosaicNode | undefined, out: string[] = []): string[] {
  if (!node) return out
  if (node.type === 'leaf') out.push(node.slot)
  else {
    treeSlots(node.a, out)
    treeSlots(node.b, out)
  }
  return out
}

function removeSlot(node: MosaicNode, slot: string): MosaicNode | null {
  if (node.type === 'leaf') return node.slot === slot ? null : node
  const a = removeSlot(node.a, slot)
  const b = removeSlot(node.b, slot)
  if (a === null) return b
  if (b === null) return a
  return { ...node, a, b }
}

function splitAtLeaf(node: MosaicNode, targetSlot: string, source: MosaicLeaf, side: Side): MosaicNode {
  if (node.type === 'leaf') {
    if (node.slot !== targetSlot) return node
    if (side === 'left') return { type: 'split', dir: 'row', ratio: 0.5, a: source, b: node }
    if (side === 'right') return { type: 'split', dir: 'row', ratio: 0.5, a: node, b: source }
    if (side === 'top') return { type: 'split', dir: 'col', ratio: 0.5, a: source, b: node }
    if (side === 'bottom') return { type: 'split', dir: 'col', ratio: 0.5, a: node, b: source }
    return node
  }
  return { ...node, a: splitAtLeaf(node.a, targetSlot, source, side), b: splitAtLeaf(node.b, targetSlot, source, side) }
}

function swapSlots(node: MosaicNode, a: string, b: string): MosaicNode {
  if (node.type === 'leaf') {
    if (node.slot === a) return leaf(b)
    if (node.slot === b) return leaf(a)
    return node
  }
  return { ...node, a: swapSlots(node.a, a, b), b: swapSlots(node.b, a, b) }
}

export function setRatioAtPath(node: MosaicNode, path: string, ratio: number): MosaicNode {
  if (path === '') {
    return node.type === 'split' ? { ...node, ratio: clamp(ratio, 0.05, 0.95) } : node
  }
  if (node.type !== 'split') return node
  const head = path[0]
  const rest = path.slice(1)
  if (head === 'a') return { ...node, a: setRatioAtPath(node.a, rest, ratio) }
  return { ...node, b: setRatioAtPath(node.b, rest, ratio) }
}

export function reconcileTree(tree: MosaicNode | undefined, aliases: string[]): MosaicNode {
  if (!aliases.length) return leaf('cam1')
  if (!tree) return defaultMosaicTree(aliases)
  let next: MosaicNode | undefined = tree
  for (const slot of treeSlots(next)) {
    if (!aliases.includes(slot)) {
      next = next ? removeSlot(next, slot) ?? undefined : undefined
    }
  }
  for (const alias of aliases) {
    if (!treeSlots(next).includes(alias)) {
      next = next ? { type: 'split', dir: 'row', ratio: 0.7, a: next, b: leaf(alias) } : leaf(alias)
    }
  }
  return next ?? defaultMosaicTree(aliases)
}

export function dockCamera(tree: MosaicNode, sourceAlias: string, targetAlias: string, side: Side): MosaicNode {
  if (sourceAlias === targetAlias) return tree
  // Dock to the outer edge of the whole layout → wrap the entire tree in a new split,
  // giving the camera a full-height column (left/right) or full-width row (top/bottom).
  if (targetAlias === ROOT_TARGET) {
    if (side === 'center') return tree
    const rest = removeSlot(tree, sourceAlias)
    if (!rest) return tree
    const src = leaf(sourceAlias)
    const dir: 'row' | 'col' = side === 'left' || side === 'right' ? 'row' : 'col'
    const sourceFirst = side === 'left' || side === 'top'
    return { type: 'split', dir, ratio: 0.5, a: sourceFirst ? src : rest, b: sourceFirst ? rest : src }
  }
  if (side === 'center') {
    return swapSlots(tree, sourceAlias, targetAlias)
  }
  const withoutSource = removeSlot(tree, sourceAlias)
  if (!withoutSource) return tree
  return splitAtLeaf(withoutSource, targetAlias, leaf(sourceAlias), side)
}

export function parseMosaic(raw: string | undefined): MosaicNode | undefined {
  if (!raw) return undefined
  try {
    return normalizeNode(JSON.parse(raw))
  } catch {
    return undefined
  }
}

function normalizeNode(value: unknown): MosaicNode | undefined {
  if (!value || typeof value !== 'object') return undefined
  const node = value as Record<string, unknown>
  if (node.type === 'leaf' && typeof node.slot === 'string') return leaf(node.slot)
  if (node.type === 'split' && (node.dir === 'row' || node.dir === 'col')) {
    const a = normalizeNode(node.a)
    const b = normalizeNode(node.b)
    if (!a || !b) return undefined
    const ratio = typeof node.ratio === 'number' ? clamp(node.ratio, 0.05, 0.95) : 0.5
    return { type: 'split', dir: node.dir, ratio, a, b }
  }
  return undefined
}


function clamp(value: number, min: number, max: number) {
  return Number.isFinite(value) ? Math.min(max, Math.max(min, value)) : min
}
