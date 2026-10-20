import test from 'node:test'
import assert from 'node:assert/strict'
import { existsSync } from 'node:fs'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { gunzipSync } from 'node:zlib'
import { precompressDir } from '../precompress.ts'

test('precompressDir gzips compressible files and skips the rest', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'precompress-'))
  try {
    await mkdir(join(dir, 'assets'))
    const js = 'export const x = 1;\n'.repeat(200)
    await writeFile(join(dir, 'assets/index-AbC1.js'), js)
    await writeFile(join(dir, 'index.html'), '<html>' + '<p>Kamera</p>'.repeat(200) + '</html>')
    await writeFile(join(dir, 'assets/font-AbC1.woff2'), Buffer.alloc(4096))
    await writeFile(join(dir, 'tiny.css'), 'a{}')

    await precompressDir(dir)

    const gz = join(dir, 'assets/index-AbC1.js.gz')
    assert.equal(gunzipSync(await readFile(gz)).toString(), js)
    assert.ok(existsSync(join(dir, 'index.html.gz')))
    assert.ok(!existsSync(join(dir, 'assets/font-AbC1.woff2.gz')))
    assert.ok(!existsSync(join(dir, 'tiny.css.gz')))
  } finally {
    await rm(dir, { recursive: true, force: true })
  }
})
