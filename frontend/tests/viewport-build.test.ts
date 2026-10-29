import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtemp, readFile, readdir, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { build } from 'vite'

test('production CSS keeps viewport fallbacks for tablets without dvh support', async () => {
  const outDir = await mkdtemp(join(tmpdir(), 'viewer-viewport-'))
  try {
    await build({ root: fileURLToPath(new URL('../', import.meta.url)), logLevel: 'silent', build: { outDir } })
    const assets = join(outDir, 'assets')
    const css = (await Promise.all((await readdir(assets)).filter(name => name.endsWith('.css')).map(name => readFile(join(assets, name), 'utf8')))).join('\n')
    const viewer = css.match(/\.viewer-root\[data-v-[^\]]+\]\{([^}]+)\}/)?.[1] ?? ''
    // An older browser ignores dvh. With only an auto height left, the grid's
    // absolutely positioned cameras give its first row zero intrinsic height.
    const legacy = viewer.replace(/(?:min-|max-)?height:[^;{}]*dvh;?/g, '')
    assert.match(legacy, /(?:^|;)height:100vh(?:;|$)/, 'camera stage must retain a definite viewport height')
    assert.match(viewer, /height:100dvh/, 'modern tablets still use the dynamic viewport')
    const canvas = css.match(/\.shell-bleed[^{}]*\.canvas\{([^}]+)\}/)?.[1] ?? ''
    assert.match(canvas, /min-height:100vh/, 'viewer shell must also retain its fallback')
  } finally {
    await rm(outDir, { recursive: true, force: true })
  }
})
