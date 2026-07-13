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
